/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

*/

///   commented with "///" is by Zhiguo,  22/09/2017
///   Other comments can use different symbols

package pbft

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"math/rand" //=> Agzs
	"sort"
	"sync" ////shaoxiaobei
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/hibe"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/op/go-logging"

	///	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/util/events" ///copy util/events from fabric as an independent tool  --Zhiguo
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/minerapi"
	"github.com/ethereum/go-ethereum/peerapi"
	"github.com/ethereum/go-ethereum/synch"

	///	"github.com/golang/protobuf/proto"
	"github.com/spf13/viper"
	"github.com/vipally/gx/unsafe"

	"github.com/ethereum/go-ethereum/core"
)

// =============================================================================
// init
// =============================================================================

var logger *logging.Logger // package-level logger
var BestPeer interface{}
var hibefinished bool
var hibestart bool

func init() {
	logger = logging.MustGetLogger("consensus/pbft")
}

const (
	// UnreasonableTimeout is an ugly thing, we need to create timers, then stop them before they expire, so use a large timeout
	UnreasonableTimeout = 100 * time.Hour
)

// =============================================================================
// custom interfaces and structure definitions
// =============================================================================
////xiaobei
type checkpointMessage struct {
	seqNo uint64
	id    []byte
}

type stateUpdateTarget struct {
	checkpointMessage
	replicas []uint64
}

////
// Event Types
// workEvent is a temporary type, to inject work
type workEvent func()

// viewChangeTimerEvent is sent when the view change timer expires
type viewChangeTimerEvent struct{}

/// // execDoneEvent is sent when an execution completes
////xiaobei
type ExecDoneEvent struct{}

type CommittedEvent struct{} ////xiaobei 1.2

var CommittedBlock = make(chan *types.Block, 1) ////xiaobei --12.27

var StateTransferFlag bool = false ////xiaobei 1.9

// type Pbftmessage struct { ////--xiaobei 11.2
// 	Sender uint64
// 	Msg    *types.PbftMessage
// }

// viewChangedEvent is sent when the view change timer expires
type viewChangedEvent struct{}

// viewChangeResendTimerEvent is sent when the view change resend timer expires
type viewChangeResendTimerEvent struct{}

// returnBlockMsgEvent is sent by pbft when we are forwarded a request
type returnBlockMsgEvent *types.Block //=>TODO. --Agzs

// nullRequestEvent provides "keep-alive" null requests
type nullRequestEvent struct{}

/// innerStack is replaced by ProtocolManager and SignFn of Clique and Ethereum   ---Zhiguo
/* // Unless otherwise noted, all methods consume the PBFT thread, and should therefore
// not rely on PBFT accomplishing any work while that thread is being held
type innerStack interface {
	broadcast(msgPayload []byte)
	unicast(msgPayload []byte, receiverID uint64) (err error)
// execute,getState not needed---Zhiguo
//	execute(seqNo uint64, block *BlockMsg) // This is invoked on a separate thread
//	getState() []byte

// the following two not needed either?  not sure about getLastSeqNo  ---Zhiguo
//	getLastSeqNo() (uint64, error)
//	skipTo(seqNo uint64, snapshotID []byte, peers []uint64)

	sign(msg []byte) ([]byte, error)
	verify(senderID uint64, signature []byte, message []byte) error

// validateState not needed ---Zhiguo
//	invalidateState()
//	validateState()
} */

//=> --Agzs
// This structure is used for incoming PBFT bound messages
// type pbftMessage struct {
// 	sender uint64
// 	msg    *types.PbftMessage
// }

type pbftCore struct {

	//=> copy signer signFn lock from PBFT, used for signing --Agzs
	signer common.Address // Ethereum address of the signing key
	signFn SignerFn       // Signer function to authorize hashes with
	lock   sync.RWMutex   // Protects the signer fields

	//=> databaseHelper copied from fabric/consensus/helper/persist used for pset and qset in viewchange --Agzs
	//=> dbHelper *databaseHelper
	helper *Helper //=> copy some func from fabric, and add databaseHelper to Helper. --Agzs

	///	executing    bool // signals that application is executing    commented by Zhiguo
	commChan     chan *types.PbftMessage
	finishedChan chan struct{}

	///	pm	*eth.ProtocolManager	// for communication. 30/09  Zhiguo

	// internal data
	//=>internalLock sync.Mutex //=> not be userd, commented by Agzs
	///	executing    bool // signals that application is executing    commented by Zhiguo

	//=>idleChan   chan struct{} // Used to detect idleness for testing //=> not be userd, commented by Agzs
	//=>injectChan chan func()   // Used as a hack to inject work onto the PBFT thread, to be removed eventually //=> not be userd, commented by Agzs

	//	consumer innerStack

	///	pm	*eth.ProtocolManager	// for communication. 30/09  Zhiguo
	// PBFT data
	activeView    bool              // view change happening
	byzantine     bool              // whether this node is intentionally acting as Byzantine; useful for debugging on the testnet
	f             int               // max. number of faults we can tolerate
	N             int               // max.number of validators in the network
	h             uint64            // low watermark
	id            uint64            // replica ID; PBFT `i`
	K             uint64            // checkpoint period
	logMultiplier uint64            // use this value to calculate log size : k*logMultiplier
	L             uint64            // log size
	LastExec      *uint64           // last block we executed   commented by Zhiguo
	replicaCount  int               // number of replicas; PBFT `|R|`
	seqNo         uint64            // PBFT "n", strictly monotonic increasing sequence number
	view          uint64            // current view
	chkpts        map[uint64]string // state checkpoints; map LastExec to global hash
	pset          map[uint64]*types.ViewChange_PQ
	qset          map[qidx]*types.ViewChange_PQ

	skipInProgress    bool               // Set when we have detected a fall behind scenario until we pick a new starting point
	stateTransferring bool               // Set when state transfer is executing  //// --xiaobei
	highStateTarget   *stateUpdateTarget // Set to the highest weak checkpoint cert we have observed --xiaobei
	hChkpts           map[uint64]uint64  // highest checkpoint sequence number observed for each replica

	currentExec *uint64 // currently executing block --xiaobei

	timerActive        bool          // is the timer running?
	vcResendTimer      events.Timer  // timer triggering resend of a view change
	newViewTimer       events.Timer  // timeout triggering a view change
	requestTimeout     time.Duration // progress timeout for requests
	vcResendTimeout    time.Duration // timeout before resending view change
	newViewTimeout     time.Duration // progress timeout for new views
	newViewTimerReason string        // what triggered the timer
	lastNewViewTimeout time.Duration // last timeout we used during this view change
	broadcastTimeout   time.Duration // progress timeout for broadcast

	outstandingBlocks map[string]*types.Block // track whether we are waiting for request batches to execute

	nullRequestTimer   events.Timer  // timeout triggering a null request
	nullRequestTimeout time.Duration // duration for this timeout
	viewChangePeriod   uint64        // period between automatic view changes
	viewChangeSeqNo    uint64        // next seqNo to perform view change

	missingReqBatches map[string]bool // for all the assigned, non-checkpointed request batches we might be missing during view-change
	//=> missingReqBatches -> missingReqBlockes --Agzs

	blockStore      map[string]*types.Block     // track request batches   BlockMsg --> Block. --Zhiguo
	certStore       map[msgID]*msgCert          // track quorum certificates for requests
	checkpointStore map[types.Checkpoint]bool   // track checkpoints as set  ////xiaobei
	viewChangeStore map[vcidx]*types.ViewChange // track view-change messages
	newViewStore    map[uint64]*types.NewView   // track last new-view we received or sent

	valid bool //// Whether we believe the state is up to date --xiaobei

	testMsgs   map[uint32]*TestMsg
	commitTest map[uint32]*TestMsg
}

var PBFTCore *pbftCore ////xiaobei --12.18

var ChangeViewFlag bool = false     ////xiaobei 1.21
var SendViewChangeFlag bool = false ////xiaobei 1.21

type qidx struct {
	d string
	n uint64
}

type msgID struct { // our index through certStore
	v uint64
	n uint64
}

type msgCert struct {
	digest      string ////block’s hash
	prePrepare  *types.PrePrepare
	sentPrepare bool
	prepare     []*types.Prepare
	sentCommit  bool
	commit      []*types.Commit
}

type vcidx struct {
	v  uint64
	id uint64
}

type sortableUint64Slice []uint64

func (a sortableUint64Slice) Len() int {
	return len(a)
}
func (a sortableUint64Slice) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a sortableUint64Slice) Less(i, j int) bool {
	return a[i] < a[j]
}

// =============================================================================
// constructors
// =============================================================================
func newPbftCore(id uint64, config *viper.Viper, etf events.TimerFactory, com chan *types.PbftMessage, fin chan struct{}) *pbftCore {
	var err error
	instance := &pbftCore{}
	instance.id = id

	instance.commChan = com
	instance.finishedChan = fin
	instance.helper = NewHelper() //=>add databaseHelper to Helper. --Agzs

	instance.newViewTimer = etf.CreateTimer()
	instance.vcResendTimer = etf.CreateTimer()
	instance.nullRequestTimer = etf.CreateTimer()

	instance.N = config.GetInt("general.N")
	instance.f = config.GetInt("general.f")
	if instance.f*3+1 > instance.N {
		panic(fmt.Sprintf("need at least %d enough replicas to tolerate %d byzantine faults, but only %d replicas configured", instance.f*3+1, instance.f, instance.N))
	}

	instance.K = uint64(config.GetInt("general.K"))

	instance.logMultiplier = uint64(config.GetInt("general.logmultiplier"))
	if instance.logMultiplier < 2 {
		panic("Log multiplier must be greater than or equal to 2")
	}
	instance.L = instance.logMultiplier * instance.K // log size
	instance.viewChangePeriod = uint64(config.GetInt("general.viewchangeperiod"))
	instance.viewChangePeriod = uint64(30)
	instance.byzantine = config.GetBool("general.byzantine")

	instance.requestTimeout, err = time.ParseDuration(config.GetString("general.timeout.request"))
	if err != nil {
		panic(fmt.Errorf("Cannot parse request timeout: %s", err))
	}
	instance.vcResendTimeout, err = time.ParseDuration(config.GetString("general.timeout.resendviewchange"))
	if err != nil {
		panic(fmt.Errorf("Cannot parse request timeout: %s", err))
	}
	instance.newViewTimeout, err = time.ParseDuration(config.GetString("general.timeout.viewchange"))
	instance.newViewTimeout = 30 * time.Second
	if err != nil {
		panic(fmt.Errorf("Cannot parse new view timeout: %s", err))
	}
	instance.nullRequestTimeout, err = time.ParseDuration(config.GetString("general.timeout.nullrequest"))
	if err != nil {
		instance.nullRequestTimeout = 0
	}
	instance.broadcastTimeout, err = time.ParseDuration(config.GetString("general.timeout.broadcast"))
	if err != nil {
		panic(fmt.Errorf("Cannot parse new broadcast timeout: %s", err))
	}

	instance.activeView = true
	instance.replicaCount = instance.N

	///	logger.Infof("PBFT type = %T", instance.consumer)
	logger.Infof("PBFT Max number of validating peers (N) = %v", instance.N)
	logger.Infof("PBFT Max number of failing peers (f) = %v", instance.f)
	logger.Infof("PBFT byzantine flag = %v", instance.byzantine)
	logger.Infof("PBFT request timeout = %v", instance.requestTimeout)
	logger.Infof("PBFT view change timeout = %v", instance.newViewTimeout)
	logger.Infof("PBFT Checkpoint period (K) = %v", instance.K)
	logger.Infof("PBFT broadcast timeout = %v", instance.broadcastTimeout)
	logger.Infof("PBFT Log multiplier = %v", instance.logMultiplier)
	logger.Infof("PBFT log size (L) = %v", instance.L)
	if instance.nullRequestTimeout > 0 {
		logger.Infof("PBFT null requests timeout = %v", instance.nullRequestTimeout)
	} else {
		logger.Infof("PBFT null requests disabled")
	}
	if instance.viewChangePeriod > 0 {
		logger.Infof("PBFT view change period = %v", instance.viewChangePeriod)
	} else {
		logger.Infof("PBFT automatic view change disabled")
	}

	// init the logs
	instance.certStore = make(map[msgID]*msgCert)

	instance.blockStore = make(map[string]*types.Block) //=> add blcokStore. --Agzs. key is Block‘s hash. --xiaobei
	///	instance.blockStore = make(map[string]*BlockMsg)
	instance.checkpointStore = make(map[types.Checkpoint]bool) ////store checkpoints --xiaobei
	instance.chkpts = make(map[uint64]string)                  ////store checkpoints' state --xiaobei
	instance.viewChangeStore = make(map[vcidx]*types.ViewChange)
	instance.pset = make(map[uint64]*types.ViewChange_PQ)
	instance.qset = make(map[qidx]*types.ViewChange_PQ)
	instance.newViewStore = make(map[uint64]*types.NewView)

	instance.testMsgs = make(map[uint32]*TestMsg)
	instance.commitTest = make(map[uint32]*TestMsg)
	// initialize state transfer
	instance.hChkpts = make(map[uint64]uint64) //// store checkpoints' seqNo which exceed high watermark,key is Checkpoint.ReplicaId,value is checkpoint.SequenceNumber--xiaobei

	instance.chkpts[0] = "XXX GENESIS" ////recover the initialization of checkpoints --xiaobei

	instance.lastNewViewTimeout = instance.newViewTimeout
	instance.outstandingBlocks = make(map[string]*types.Block) ////key is block‘s hash --xiaobei
	///	instance.outstandingReqBatches = make(map[string]*BlockMsg)
	instance.missingReqBatches = make(map[string]bool) //=> --Agzs
	instance.restoreState()                            //=> --Agzs
	instance.viewChangeSeqNo = ^uint64(0)              // infinity
	instance.updateViewChangeSeqNo()

	PBFTCore = instance ////xiaobei 1.8

	return instance
}

// close tears down resources opened by newPbftCore
func (instance *pbftCore) close() {
	instance.newViewTimer.Halt()
	instance.nullRequestTimer.Halt()
}

// allow the view-change protocol to kick-off when the timer expires
func (instance *pbftCore) ProcessEvent(e events.Event) events.Event {
	var err error
	logger.Debugf("Replica %d processing event", instance.id)

	switch et := e.(type) {
	case viewChangeTimerEvent:
		logger.Infof("Replica %d view change timer expired, sending view change: %s", instance.id, instance.newViewTimerReason)
		instance.timerActive = false
		instance.sendViewChange()
	// case *pbftMessage:
	// 	return pbftMessageEvent(*et)

	// case pbftMessageEvent:
	// 	msg := et
	// 	logger.Debugf("Replica %d received incoming message from %v", instance.id, msg.sender)
	// 	next, err := instance.recvMsg(msg.msg, msg.sender)
	// 	if err != nil {
	// 		break
	// 	}
	// 	return next
	case *types.PbftMessage: //=>--Agzs
		msg := et
		logger.Debugf("Replica %d received incoming message from replica %v", instance.id, msg.GetSender())
		next, err := instance.recvMsg(msg, msg.GetSender())
		if err != nil {
			break
		}
		return next
	case stateUpdateEvent: ////restore stateUpdateEvent. --xiaobei
		logger.Infof("Pocessing a stateUpdateEvent")
		//synch.Sync.Synchronise(synch.Sync.peers.BestPeer()) ////xiaobei 1.8
		////===========>xiaobei 1.9
		StateUpdatedEvent := &stateUpdatedEvent{
			chkpt:  et.tag.(*checkpointMessage),
			target: et.BlockchainInfo,
		}
		update := StateUpdatedEvent.chkpt
		instance.stateTransferring = false
		// If state transfer did not complete successfully, or if it did not reach our low watermark, do it again
		if StateUpdatedEvent.target == nil || update.seqNo < instance.h {
			if StateUpdatedEvent.target == nil {
				logger.Warningf("Replica %d attempted state transfer target was not reachable (%v)", instance.id, StateUpdatedEvent.chkpt)
			} else {
				logger.Warningf("Replica %d recovered to seqNo %d but our low watermark has moved to %d", instance.id, update.seqNo, instance.h)
			}

			if instance.highStateTarget == nil {
				logger.Debugf("Replica %d has no state targets, cannot resume state transfer yet", instance.id)
			} else if update.seqNo < instance.highStateTarget.seqNo {
				logger.Debugf("Replica %d has state target for %d, transferring", instance.id, instance.highStateTarget.seqNo)
				instance.retryStateTransfer(nil)
			} else {
				logger.Debugf("Replica %d has no state target above %d, highest is %d", instance.id, update.seqNo, instance.highStateTarget.seqNo)
			}
			return nil
		}
		logger.Infof("Replica %d application caught up via state transfer, LastExec now %d", instance.id, update.seqNo)
		n := update.seqNo                     ////xiaobei 1.8
		instance.LastExec = &n                ////xiaobei 1.8
		instance.moveWatermarks(update.seqNo) // The watermark movement handles moving this to a checkpoint boundary
		instance.skipInProgress = false
		StateTransferFlag = true ////xiaobei 1.9
		instance.helper.ValidateState()
		instance.Checkpoint(update.seqNo, update.id)
		instance.executeOutstanding()
		////xiaobei 1.21
		fmt.Printf("----ChangeViewFlag is\n", ChangeViewFlag)
		fmt.Printf("----SendViewChangeFlag is\n", SendViewChangeFlag)
		if ChangeViewFlag {
			if !SendViewChangeFlag {
				instance.view++
				fmt.Printf("------new view is %d", instance.view)
			} else {
				instance.activeView = true
			}
			SendViewChangeFlag = false
			ChangeViewFlag = false
		}
		////
		////===========>xiaobei 1.9
		logger.Infof("-----block synchronise start, try to handshake") ////xiaobei 1.8
		err := synch.Sync.RetryHandShake(&peerapi.Peer)                ////xiaobei 1.9
		if err == nil {
			synch.Sync.Synchronise2(&peerapi.Peer) ////xiaobei 1.8
			//fmt.Printf("----peerapi.Peer.ID is %s",peerapi.Peer.ID)////xiaobei 1.9
			//instance.helper.StateUpdated(et.tag, et.BlockchainInfo)
			et.peers = nil
			return nil
		}
	// case stateUpdatedEvent: ////xiaobei
	// 	logger.Infof("-----stateUpdatedEvent is called") ////xiaobei 1.8
	// 	update := et.chkpt
	// 	instance.stateTransferring = false
	// 	// If state transfer did not complete successfully, or if it did not reach our low watermark, do it again
	// 	if et.target == nil || update.seqNo < instance.h {
	// 		if et.target == nil {
	// 			logger.Warningf("Replica %d attempted state transfer target was not reachable (%v)", instance.id, et.chkpt)
	// 		} else {
	// 			logger.Warningf("Replica %d recovered to seqNo %d but our low watermark has moved to %d", instance.id, update.seqNo, instance.h)
	// 		}

	// 		if instance.highStateTarget == nil {
	// 			logger.Debugf("Replica %d has no state targets, cannot resume state transfer yet", instance.id)
	// 		} else if update.seqNo < instance.highStateTarget.seqNo {
	// 			logger.Debugf("Replica %d has state target for %d, transferring", instance.id, instance.highStateTarget.seqNo)
	// 			instance.retryStateTransfer(nil)
	// 		} else {
	// 			logger.Debugf("Replica %d has no state target above %d, highest is %d", instance.id, update.seqNo, instance.highStateTarget.seqNo)
	// 		}
	// 		return nil
	// 	}
	// 	logger.Infof("Replica %d application caught up via state transfer, LastExec now %d", instance.id, update.seqNo)
	// 	n := update.seqNo                           ////xiaobei 1.8
	// 	instance.LastExec = &n                      ////xiaobei 1.8
	// 	instance.moveWatermarks(update.seqNo) // The watermark movement handles moving this to a checkpoint boundary
	// 	instance.skipInProgress = false
	// 	instance.helper.ValidateState()
	// 	instance.Checkpoint(update.seqNo, update.id)
	// 	instance.executeOutstanding()
	// ////xiaobei 1.2
	case *TestMsg: //test hibe
		err = instance.recvPrePrepareTestMsg(et)
	case *PrepareTestMsg: //test hibe
		err = instance.recvPrepareTestMsg(et)
	case *CommitTestMsg: //test hibe
		err = instance.recvCommitTestMsg(et)
	case *types.PrePrepare:
		err = instance.recvPrePrepare(et)
	case *types.Prepare:
		err = instance.recvPrepare(et)
	case *types.Commit:
		err = instance.recvCommit(et)
	case *types.Checkpoint: //recover checkpoint --xiaobei
		return instance.recvCheckpoint(et)
	case *types.ViewChange:
		return instance.recvViewChange(et)
	case *types.NewView:
		return instance.recvNewView(et)
		///	case *FetchBlockMsg:
		///		err = instance.recvFetchBlockMsg(et)
		///	case returnBlockMsgEvent:
		///		return instance.recvReturnBlockMsg(et)
	case CommittedEvent:
		logger.Debugf("Replica %d received committedEvent", instance.id)
		return ExecDoneEvent{}
	////
	////xiaobei 1.18
	case core.CommittedEvent2:
		logger.Infof("-------CommittedEvent2 is called") ////xiaobei 1.18
		return CommittedEvent{}
	////
	case ExecDoneEvent: ////add execDoneEvent.  --xiaobei
		logger.Infof("execDoneEvent has been called")
		instance.execDoneSync()
		log.Info("case execDoneEvent:") //=>test. --Agzs
		if instance.skipInProgress {
			instance.retryStateTransfer(nil)
		}
		//We will delay new view processing sometimes
		return instance.processNewView() ////--xiaobei 11.23
	case nullRequestEvent:
		logger.Infof("nullRequestEvent is called.") ////test --xiaobei 11.9
		instance.nullRequestHandler()
	case workEvent:
		et() // Used to allow the caller to steal use of the main thread, to be removed
	case viewChangeQuorumEvent:
		logger.Debugf("Replica %d received view change quorum, processing new view", instance.id)
		core.ViewChangeFlag = true                   ////xiaobei 1.29
		logger.Info("------set ViewChangeFlag true") ////xiaobei 1.29
		if instance.primary(instance.view) == instance.id {
			logger.Debugf("Replica %d is primary will send new view", instance.id) ////test --xiaobei 11.9
			return instance.sendNewView()
		}
		return instance.processNewView()
	case viewChangedEvent: //=> add --Agzs
		// No-op, processed by plugins if needed
		//=>instance.blockStore = nil
		instance.blockStore = make(map[string]*types.Block) ////--xiaobei 11.9

		// Outstanding reqs doesn't make sense for batch, as all the requests in a batch may be processed
		// in a different batch, but PBFT core can't see through the opaque structure to see this
		// so, on view change, clear it out
		instance.outstandingBlocks = make(map[string]*types.Block)

		////xiaobei --12.13
		b := minerapi.Minerapi.Stop()
		if b {
			logger.Infof("-----miner.stop() successful-----")
		} else {
			logger.Infof("-----miner.stop() failed----")
		}

		time.Sleep(time.Second) ////xiaobei --12.28
		log.Info("---stop 1s")
		err := minerapi.Minerapi.Start(nil)
		if err != nil {
			fmt.Println("----miner.start() err is", err)
		} else {
			logger.Infof("-----miner.start() successful----")
		}
		core.ViewChangeFlag = false
		logger.Info("-------Set ViewChangeFlag false")
		logger.Debugf("Replica %d batch thread recognizing new view and restart mining", instance.id)
		////

	case viewChangeResendTimerEvent:
		if instance.activeView {
			logger.Warningf("Replica %d had its view change resend timer expire but it's in an active view, this is benign but may indicate a bug", instance.id)
			return nil
		}
		logger.Debugf("Replica %d view change resend timer expired before view change quorum was reached, resending", instance.id)
		instance.view-- // sending the view change increments this
		return instance.sendViewChange()
	default:
		logger.Warningf("Replica %d received an unknown message type %T", instance.id, et)
	}

	if err != nil {
		logger.Warning(err.Error())
	}

	return nil
}

// =============================================================================
// helper functions for PBFT
// =============================================================================

// Given a certain view n, what is the expected primary?
func (instance *pbftCore) primary(n uint64) uint64 {
	return n % uint64(instance.replicaCount)
}

// Is the sequence number between watermarks?
//=> instance.h < n <= instance.h + instance.L --Agzs
func (instance *pbftCore) inW(n uint64) bool {
	return n-instance.h > 0 && n-instance.h <= instance.L
}

// Is the view right? And is the sequence number between watermarks?
func (instance *pbftCore) inWV(v uint64, n uint64) bool {
	////xiaobei 1.21
	if instance.view != v {
		ChangeViewFlag = true
	}
	////
	return instance.view == v && instance.inW(n)
}

// Given a digest/view/seq, is there an entry in the certLog?
// If so, return it. If not, create it.
func (instance *pbftCore) getCert(v uint64, n uint64) (cert *msgCert) {
	idx := msgID{v, n}
	cert, ok := instance.certStore[idx]
	if ok {
		return
	}

	cert = &msgCert{}
	instance.certStore[idx] = cert
	return
}

// =============================================================================
// preprepare/prepare/commit quorum checks
// =============================================================================

// intersectionQuorum returns the number of replicas that have to
// agree to guarantee that at least one correct replica is shared by
// two intersection quora
func (instance *pbftCore) intersectionQuorum() int {
	return (instance.N + instance.f + 2) / 2
}

// allCorrectReplicasQuorum returns the number of correct replicas (N-f)
func (instance *pbftCore) allCorrectReplicasQuorum() int {
	return (instance.N - instance.f)
}

func (instance *pbftCore) prePrepared(digest string, v uint64, n uint64) bool {
	///	_, mInLog := instance.blockStore[digest]

	///	if digest != "" && !mInLog {
	///		return false
	///	}

	if q, ok := instance.qset[qidx{digest, n}]; ok && q.View == v {
		return true
	}

	cert := instance.certStore[msgID{v, n}]
	if cert != nil {
		p := cert.prePrepare
		if p != nil && p.View == v && p.SequenceNumber == n && string(p.BlockHash[:]) == digest {
			return true
		}
	}
	logger.Debugf("Replica %d does not have view=%d/seqNo=%d pre-prepared",
		instance.id, v, n)
	return false
}

func (instance *pbftCore) prepared(digest string, v uint64, n uint64) bool {
	if !instance.prePrepared(digest, v, n) {
		return false
	}

	if p, ok := instance.pset[n]; ok && p.View == v && p.BlockHash.Str() == digest { ////--xiaobei 11.21
		return true
	}

	quorum := 0
	cert := instance.certStore[msgID{v, n}]
	if cert == nil {
		return false
	}

	for _, p := range cert.prepare {
		if p.View == v && p.SequenceNumber == n && string(p.BlockHash[:]) == digest {
			quorum++
		}
	}

	logger.Debugf("Replica %d prepare count for view=%d/seqNo=%d: %d",
		instance.id, v, n, quorum)

	return quorum >= instance.intersectionQuorum()-1
}

func (instance *pbftCore) committed(digest string, v uint64, n uint64) bool {
	if !instance.prepared(digest, v, n) {
		return false
	}

	quorum := 0
	cert := instance.certStore[msgID{v, n}]
	if cert == nil {
		return false
	}

	for _, p := range cert.commit {
		if p.View == v && p.SequenceNumber == n {
			quorum++
		}
	}

	logger.Debugf("Replica %d commit count for view=%d/seqNo=%d: %d",
		instance.id, v, n, quorum)

	return quorum >= instance.intersectionQuorum()
}

// =============================================================================
// receive methods
// =============================================================================

func (instance *pbftCore) nullRequestHandler() {
	if !instance.activeView {
		logger.Infof("!instance.activeView, will return") ////test --xiaobei 11.9
		return
	}

	if instance.primary(instance.view) != instance.id {
		// backup expected a null request, but primary never sent one
		logger.Infof("Replica %d null request timer expired, sending view change", instance.id) ////test --xiaobei 11.9
		instance.sendViewChange()
	} else {
		// time for the primary to send a null request
		// pre-prepare with null digest
		logger.Infof("Primary %d null request timer expired, sending null request", instance.id) ////test --xiaobei 11.9
		instance.sendPrePrepare(nil)
	}
}

func (instance *pbftCore) recvMsg(msg *types.PbftMessage, senderID uint64) (interface{}, error) {
	// if block := msg.GetBlockMsg(); block != nil {
	// 	return block, nil
	// } else

	//log.Info("recvMsg() test", "sendID", senderID, "code", msg.GetPayloadCode(), "payload", reflect.ValueOf(msg.Payload).Type()) //=>test. --Agzs

	if preprep := msg.GetPrePrepare(); preprep != nil {
		////xiaobei 10.16
		//if block := preprep.GetBlockMsg(); block != nil {
		//	return block, nil
		//}

		if senderID != preprep.ReplicaId {
			return nil, fmt.Errorf("Sender ID included in pre-prepare message (%v) doesn't match ID corresponding to the receiving stream (%v)", preprep.ReplicaId, senderID)
		}
		return preprep, nil
	} else if prep := msg.GetPrepare(); prep != nil {
		if senderID != prep.ReplicaId {
			return nil, fmt.Errorf("Sender ID included in prepare message (%v) doesn't match ID corresponding to the receiving stream (%v)", prep.ReplicaId, senderID)
		}
		return prep, nil
	} else if commit := msg.GetCommit(); commit != nil {
		if senderID != commit.ReplicaId {
			return nil, fmt.Errorf("Sender ID included in commit message (%v) doesn't match ID corresponding to the receiving stream (%v)", commit.ReplicaId, senderID)
		}
		return commit, nil
	} else if chkpt := msg.GetCheckpoint(); chkpt != nil {
		if senderID != chkpt.ReplicaId {
			return nil, fmt.Errorf("Sender ID included in checkpoint message (%v) doesn't match ID corresponding to the receiving stream (%v)", chkpt.ReplicaId, senderID)
		}
		return chkpt, nil
	} else if vc := msg.GetViewChange(); vc != nil {
		if senderID != vc.ReplicaId {
			return nil, fmt.Errorf("Sender ID included in view-change message (%v) doesn't match ID corresponding to the receiving stream (%v)", vc.ReplicaId, senderID)
		}
		return vc, nil
	} else if nv := msg.GetNewView(); nv != nil {
		if senderID != nv.ReplicaId {
			return nil, fmt.Errorf("Sender ID included in new-view message (%v) doesn't match ID corresponding to the receiving stream (%v)", nv.ReplicaId, senderID)
		}
		return nv, nil
		// } else if fr := msg.GetFetchBlockMsg(); fr != nil {
		// 	if senderID != fr.ReplicaId {
		// 		return nil, fmt.Errorf("Sender ID included in fetch-request-batch message (%v) doesn't match ID corresponding to the receiving stream (%v)", fr.ReplicaId, senderID)
		// 	}
		// 	return fr, nil
		// } else if block := msg.GetReturnBlockMsg(); block != nil {
		// 	// it's ok for sender ID and replica ID to differ; we're sending the original request message
		// 	return returnBlockMsgEvent(block), nil
	} else if pbftmsg, ok := msg.Payload.(*TestMsg); ok { //test hibe
		return pbftmsg, nil
	} else if pbftmsg, ok := msg.Payload.(*PrepareTestMsg); ok { //test hibe
		return pbftmsg, nil
	} else if pbftmsg, ok := msg.Payload.(*CommitTestMsg); ok { //test hibe
		return pbftmsg, nil
	}
	return nil, fmt.Errorf("Invalid message: %v", msg)
}

//test hibe
func (instance *pbftCore) recvRequestTestMsg(msg *TestMsg) error {
	m := *msg
	mm := &m
	if _, ok := instance.testMsgs[msg.NodeIndex]; !ok {
		instance.testMsgs[msg.NodeIndex] = mm
	}

	if instance.primary(instance.view) == instance.id {
		fmt.Printf("Primary [%d] received a TestMsg\n", instance.id)
	} else {
		fmt.Printf("Replica [%d] received a TestMsg\n", instance.id)
	}

	if instance.primary(instance.view) == instance.id {
		instance.sendPrePrepareTestMsg(msg)
	}

	return nil
}

//test hibe
func (instance *pbftCore) sendPrePrepareTestMsg(msg *TestMsg) {
	if msg == nil {
		return
	}

	m := *msg
	mm := &m
	n := instance.seqNo + 1
	mm.Seq = n

	pbftMsg := &types.PbftMessage{
		Sender:      instance.id,
		PayloadCode: types.PrePrepareTestMsg,
		Payload:     mm,
	}

	instance.commChan <- pbftMsg
	fmt.Printf("sender[%d] send PrePreparetestMsg, seqNo is[%d]\n", instance.id, n)

}

//test hibe
func (instance *pbftCore) recvPrePrepareTestMsg(msg *TestMsg) error {

	if msg == nil {
		return nil
	}
	m := *msg
	mm := &m

	if _, ok := instance.testMsgs[msg.NodeIndex]; ok {
		return nil
	}
	if node.NodeIndex != 1 {
		if hibestart == false {
			hibestart = true
		}
		node.Start = time.Now()
	}
	instance.testMsgs[msg.NodeIndex] = mm
	fmt.Printf("replica[%d] receive preprepareTestMsg from replica[%d]\n", instance.id, msg.ReplicaID)

	if instance.primary(instance.view) != msg.ReplicaID {
		logger.Warningf("Pre-prepare from other than primary: got %d, should be %d", msg.ReplicaID, instance.primary(instance.view))
		return nil
	}

	var timer *time.Timer

	for hibe.PrivateKey == nil || hibe.MasterPubKey == nil {
		log.Info("key is nil")
		if timer == nil {
			timer = time.NewTimer(5 * time.Second)
		}
		<-timer.C
		timer.Reset(5 * time.Second)
	}
	if timer != nil {
		timer.Stop()
	}
	start := time.Now()
	signature := hibe.ShadowSign(hibe.PrivateKey, hibe.MasterPubKey, []byte(msg.Str), hibe.Random)
	end := time.Now()
	if node.ResultFile != nil {
		wt := bufio.NewWriter(node.ResultFile)
		str := fmt.Sprintf("time for node %d ShadowSign  is :%v:\n", node.NodeIndex, end.Sub(start))
		_, err := wt.WriteString(str)
		if err != nil {
			log.Error("write error")
		}
		wt.Flush()
	}

	testMsg := &TestMsg{
		Str:       msg.Str,
		Signature: signature.SIGToBytes(),
		ReplicaID: instance.id,
		Seq:       msg.Seq,
		View:      msg.View,
		NodeIndex: hibe.Index,
	}

	prepareMsg := &PrepareTestMsg{
		TestMsg: testMsg,
	}

	pbftMsg := &types.PbftMessage{
		Sender:      instance.id,
		PayloadCode: types.PrepareTestMsg,
		Payload:     prepareMsg,
	}

	instance.recvPrepareTestMsg(prepareMsg)

	instance.commChan <- pbftMsg
	fmt.Printf("sender[%d] broadcast PreparetestMsg \n", instance.id)
	return nil
}

//test hibe
func (instance *pbftCore) recvPrepareTestMsg(msg *PrepareTestMsg) error {
	if msg == nil || msg.TestMsg == nil {
		return nil
	}

	if _, ok := instance.testMsgs[msg.TestMsg.NodeIndex]; ok {
		return nil
	}
	instance.testMsgs[msg.TestMsg.NodeIndex] = msg.TestMsg
	fmt.Printf("replica[%d] receive prepareTestMsg from replica[%d]\n", instance.id, msg.TestMsg.ReplicaID)

	return instance.maybeSendCommitTest(msg.TestMsg)

}

//
func (instance *pbftCore) maybeSendCommitTest(msg *TestMsg) error {
	if msg == nil {
		return nil
	}
	msg.ReplicaID = instance.id
	msg.NodeIndex = hibe.Index

	if len(instance.testMsgs) >= (instance.N+instance.f)/2-1 {
		msg.NodeIndex = hibe.Index
		msg.ReplicaID = instance.id
		//fmt.Println(msg.Str)
		msg.Signature = hibe.ShadowSign(hibe.PrivateKey, hibe.MasterPubKey, []byte(msg.Str), hibe.Random).SIGToBytes()
		//fmt.Println("_______________________________________________________")
		//fmt.Println(msg.Signature)
		commit := &CommitTestMsg{
			TestMsg: msg,
		}
		instance.recvCommitTestMsg(commit)
		m := &types.PbftMessage{
			Sender:      instance.id,
			PayloadCode: types.CommitTestMsg,
			Payload:     commit,
		}
		instance.commChan <- m
		fmt.Printf("sender[%d] broadcast CommittestMsg\n", instance.id)
	}
	return nil
}

//test hibe
func (instance *pbftCore) recvCommitTestMsg(msg *CommitTestMsg) error {

	if msg == nil || msg.TestMsg == nil {
		return nil
	}
	if _, ok := instance.testMsgs[msg.TestMsg.NodeIndex]; ok {
		if _, ok := instance.commitTest[msg.TestMsg.NodeIndex]; ok {
			return nil
		}
	}
	//	instance.commitTest[msg.TestMsg.NodeIndex] = msg.TestMsg

	if _, ok := instance.commitTest[msg.TestMsg.NodeIndex]; !ok {
		instance.commitTest[msg.TestMsg.NodeIndex] = msg.TestMsg
		fmt.Printf("replica[%d] receive CommitTestMsg from replica[%d]\n", instance.id, msg.TestMsg.ReplicaID)
		//	fmt.Println(msg.TestMsg.NodeIndex)
		//fmt.Println(msg.TestMsg.Signature)
	}

	instance.maybeVerify()
	return nil
}

func (instance *pbftCore) maybeVerify() error {
	if len(instance.commitTest) < (instance.N+instance.f)/2-1 {
		log.Info("do not have enough commitmsg for pbft\n")
		fmt.Println("we has ", len(instance.commitTest), "but we need", (instance.N+instance.f)/2-1)
		return nil
	}
	if uint32(len(instance.commitTest)) < hibe.N {
		log.Info("do not have enough commitmsg for hibe\n")
		return nil
	}
	if hibefinished == true {
		return nil
	}

	var signatures []*hibe.SIG
	var indexes []int
	var i uint32
	var str string
	for _, msg := range instance.commitTest {
		if i >= hibe.N {
			break
		}
		if str == "" {
			str = msg.Str
		}
		fmt.Println(msg.Signature)
		signatures = append(signatures, msg.Signature.BytesToSIG())
		indexes = append(indexes, int(msg.NodeIndex))
		i++
	}
	start := time.Now()
	sig := hibe.SignRecon(signatures, indexes)
	end := time.Now()
	if node.ResultFile != nil {
		wt := bufio.NewWriter(node.ResultFile)
		str := fmt.Sprintf("time for node %d reconstructing signature is :%v:\n", node.NodeIndex, end.Sub(start))
		_, err := wt.WriteString(str)
		if err != nil {
			log.Error("write error")
		}
		wt.Flush()
	}

	if hibe.Verify(hibe.MasterPubKey, node.ID, []byte(str), int(hibe.Level), sig) {
		log.Info("verify hibe succeed!\n")
		node.End = time.Now()
		//	fmt.Println(node.NodeIndex, "finish time:", node.End)

		diff := node.End.Sub(node.Start)

		if node.NodeIndex == 1 {
			node.HibeFinished <- str
			fmt.Println("primary node finished")
		}

		hibefinished = true
		fmt.Println("node", node.NodeIndex, "finish verify signature")
		fmt.Println("Total time is", diff)
		if node.ResultFile != nil {
			wt := bufio.NewWriter(node.ResultFile)
			str := fmt.Sprintf("time for node %d verifying signature is:%v\n", node.NodeIndex, diff)
			_, err := wt.WriteString(str)
			if err != nil {
				log.Error("write error")
			}
			wt.Flush()
		}
	}

	return nil
}

//=>TODO. recvRequestBatch -> recvRequestBlock for blockStore. --Agzs
func (instance *pbftCore) recvRequestBlock(block *types.Block) error {
	digest := block.Hash().Str() //=>replace hash(block) --Agzs

	//=>logger.Debugf("Replica %d received a block %x", instance.id, digest)
	//	log.Info("Replica received a block", "Replica(PeerID)", instance.id, "hash", common.StringToHash(digest))

	instance.blockStore[digest] = block
	instance.outstandingBlocks[digest] = block
	instance.persistRequestBlock(digest) //=> save block whose hash equal with digest in database. --Agzs

	//digest2 := block.Hash().Bytes() ////--xiaobei
	//	logger.Debugf("Replica %d received a block %x", instance.id, digest)
	if instance.activeView {
		instance.softStartTimer(instance.requestTimeout, fmt.Sprintf("new block %x", digest))
	}
	if instance.primary(instance.view) == instance.id && instance.activeView {
		instance.nullRequestTimer.Stop()
		instance.sendPrePrepare(block)
	} else {
		//=>logger.Debugf("Replica %d is backup, not sending pre-prepare for block %x", instance.id, digest)
		//		log.Info("Replica is backup, not sending pre-prepare for block", "Replica(PeerID)", instance.id, "hash", common.StringToHash(digest))
	}
	return nil
}

///	Block is already signed by primary node, so the preprepare request should contain
/// only the finalized block   ---Zhiguo 26/09
func (instance *pbftCore) sendPrePrepare(block *types.Block) {
	if block == nil {
		return
	}
	digest := block.Hash().Bytes() /// digest is from block. --Zhiguo
	//=>logger.Debugf("Replica %d is primary, issuing pre-prepare for block %x", instance.id, digest)
	//	log.Info("Replica is primary, issuing pre-prepare for block", "Replica(PeerID)", instance.id, "hash", common.BytesToHash(digest))
	////xiaobei 10.16
	n := instance.seqNo + 1
	//	logger.Infof("digest in PrePrepare msg is %x", digest) ////test --xiaobei 11.9
	preprep := &types.PrePrepare{
		// Timestamp: &timestamp.Timestamp{
		// 	Seconds: now.Unix(),
		// 	Nanos:   int32(now.UnixNano() % 1000000000),
		// },
		View:           instance.view,
		SequenceNumber: n,
		BlockHash:      block.Hash().Bytes(),
		Block:          block,
		ReplicaId:      instance.id,
	}
	//////////////////////////////////////////////////////////////////

	for _, cert := range instance.certStore { // check for other PRE-PREPARE for same digest, but different seqNo
		if p := cert.prePrepare; p != nil {
			if p.View == instance.view && p.SequenceNumber != n && string(p.BlockHash) == string(digest) && string(digest) != "" {
				//	logger.Infof("Other pre-prepare found with same digest but different seqNo: %d instead of %d", p.SequenceNumber, n)
				return
			}
		}
	}

	if !instance.inWV(instance.view, n) || n > instance.h+instance.L/2 {
		// We don't have the necessary stable certificates to advance our watermarks
		//logger.Warningf("Primary %d not sending pre-prepare for block %x - out of sequence numbers", instance.id, digest)
		return
	}

	if n > instance.viewChangeSeqNo {
		//	logger.Info("Primary %d about to switch to next primary, not sending pre-prepare with seqno=%d", instance.id, n)
		return
	}

	//logger.Debugf("Primary %d broadcasting pre-prepare for view=%d/seqNo=%d and digest %x", instance.id, instance.view, n, digest)
	instance.seqNo = n

	cert := instance.getCert(instance.view, n)
	cert.prePrepare = preprep
	cert.digest = block.Hash().Str()
	//logger.Infof("cert.digest is %s", cert.digest) ////test --xiaobei 11.21
	instance.persistQSet() //=> add qset opreation. save in database. --Agzs
	///	instance.pm.Broadcast(&Message{Payload: &Message_PrePrepare{PrePrepare: preprep}})		///TODO: implement broadcast by pm. --Zhiguo
	////xiaobei

	msg := &types.PbftMessage{
		// PrePrepare: preprep,
		// Prepare:    nil,
		// Commit:     nil,
		// Checkpoint: nil,
		// ViewChange: nil,
		// NewView:    nil,
		//FetchBlockMsg: nil,
		Sender:      instance.id,
		PayloadCode: types.PrePrepareMsg,
		Payload:     preprep,
	} //=>change --Agzs
	instance.innerBroadcast(msg)

	//	log.Info("Replica issued pre-prepare for block", "Replica(PeerID)", instance.id, "hash", common.BytesToHash(digest)) //=>test. --Agzs
	//instance.commChan <- &types.PbftMessage{Payload: &types.PbftMessage_PrePrepare{PrePrepare: preprep}}
	instance.maybeSendCommit(string(digest), instance.view, n)
}

func (instance *pbftCore) resubmitBlockMsges() {
	if instance.primary(instance.view) != instance.id {
		return
	}

	var submissionOrder []*types.Block ////BlockMsg->Block --xiaobei

outer:
	for d, block := range instance.outstandingBlocks { ////outstandingRequestes->outstandingBlocks --xiaobei
		for _, cert := range instance.certStore {
			if cert.digest == d {
				//=>logger.Debugf("Replica %d already has certificate for block %x - not going to resubmit", instance.id, d)
				//	log.Info("Replica already has certificate for block - not going to resubmit", "Replica(PeerID)", instance.id, "hash", common.StringToHash(d))
				continue outer
			}
		}
		//=>logger.Debugf("Replica %d has detected block %x must be resubmitted", instance.id, d)
		//log.Info("Replica has detected block must be resubmitted", "Replica(PeerID)", instance.id, "hash", common.StringToHash(d))
		submissionOrder = append(submissionOrder, block)
	}

	if len(submissionOrder) == 0 {
		return
	}

	for _, block := range submissionOrder {
		// This is a request batch that has not been pre-prepared yet
		// Trigger request batch processing again
		instance.recvRequestBlock(block)
	}
}

func (instance *pbftCore) recvPrePrepare(preprep *types.PrePrepare) error {
	//logger.Debugf("Replica %d received pre-prepare from replica %d for view=%d/seqNo=%d",
	//	instance.id, preprep.ReplicaId, preprep.View, preprep.SequenceNumber)

	if !instance.activeView {
		//	logger.Debugf("Replica %d ignoring pre-prepare as we are in a view change", instance.id)
		return nil
	}

	if instance.primary(instance.view) != preprep.ReplicaId {
		//	logger.Warningf("Pre-prepare from other than primary: got %d, should be %d", preprep.ReplicaId, instance.primary(instance.view))
		return nil
	}

	//=>judge view and seqNo. --Agzs
	//logger.Infof("-----low wartermarks= %d", instance.h) ////xiaobei --1.4
	if !instance.inWV(preprep.View, preprep.SequenceNumber) {
		if preprep.SequenceNumber != instance.h && !instance.skipInProgress {
			//	logger.Warningf("Replica %d pre-prepare view different, or sequence number outside watermarks: preprep.View %d, expected.View %d, seqNo %d, low-mark %d", instance.id, preprep.View, instance.primary(instance.view), preprep.SequenceNumber, instance.h)
		} else {
			// This is perfectly normal
			//	logger.Debugf("Replica %d pre-prepare view different, or sequence number outside watermarks: preprep.View %d, expected.View %d, seqNo %d, low-mark %d", instance.id, preprep.View, instance.primary(instance.view), preprep.SequenceNumber, instance.h)
		}

		return nil
	}

	if preprep.SequenceNumber > instance.viewChangeSeqNo {
		//	logger.Info("Replica %d received pre-prepare for %d, which should be from the next primary", instance.id, preprep.SequenceNumber)
		instance.sendViewChange()
		return nil
	}

	cert := instance.getCert(preprep.View, preprep.SequenceNumber)
	if cert.digest != "" && cert.digest != string(preprep.BlockHash) { /// BatchDigest --> BlockHash. --Zhiguo
		//	logger.Warningf("Pre-prepare found for same view/seqNo but different digest: received %s, stored %s", preprep.Block, cert.digest)
		instance.sendViewChange()
		return nil
	}

	cert.prePrepare = preprep
	cert.digest = string(preprep.BlockHash) ///BatchDigest --> BlockHash. --Zhiguo

	// Store the request batch if, for whatever reason, we haven't received it from an earlier broadcast
	////if the block not in the blockStore，store the block. xiaobei 10.16
	if _, ok := instance.blockStore[string(preprep.BlockHash)]; !ok && string(preprep.BlockHash) != "" {
		digest := string(preprep.Block.Hash().Bytes())
		//	logger.Infof("digest from preprepare--digest := string(preprep.Block.Hash().Bytes())/cert.digest is %x", digest) ////test --xiaobei 11.9
		if digest != string(preprep.BlockHash) {
			//		logger.Warningf("Pre-prepare and block digest do not match: request %s, digest %s", digest, preprep.Block.String())
			return nil
		}
		instance.blockStore[digest] = preprep.Block
		//logger.Infof("block %x in preprep",digest) ////test--xiaobei 11.15
		//logger.Debugf("Replica %d storing block %x in outstanding block store", instance.id, digest)
		//	log.Info("Replica storing block in outstanding block store", "Replica(PeerID)", instance.id, "hash", common.StringToHash(digest))

		instance.outstandingBlocks[digest] = preprep.Block
		instance.persistRequestBlock(digest) //=> --Agzs
		// for key := range instance.blockStore { ////--xiaobei 11.16
		// 	logger.Infof("blockstore digest is %x", key)
		// }
	}
	////
	instance.softStartTimer(instance.requestTimeout, fmt.Sprintf("new pre-prepare for block %x", preprep.Block))
	instance.nullRequestTimer.Stop()

	if instance.primary(instance.view) != instance.id && instance.prePrepared(string(preprep.BlockHash), preprep.View, preprep.SequenceNumber) && !cert.sentPrepare {
		//	logger.Debugf("Backup %d broadcasting prepare for view=%d/seqNo=%d", instance.id, preprep.View, preprep.SequenceNumber)
		prep := &types.Prepare{
			View:           preprep.View,
			SequenceNumber: preprep.SequenceNumber,
			BlockHash:      preprep.BlockHash,
			ReplicaId:      instance.id,
		}
		cert.sentPrepare = true
		instance.persistQSet() //=> --Agzs
		instance.recvPrepare(prep)

		msg := &types.PbftMessage{
			// PrePrepare: nil,
			// Prepare:    prep,
			// Commit:     nil,
			// Checkpoint: nil,
			// ViewChange: nil,
			// NewView:    nil,
			//FetchBlockMsg: nil,
			Sender:      instance.id,
			PayloadCode: types.PrepareMsg,
			Payload:     prep,
		} //=>change --Agzs
		instance.innerBroadcast(msg)
		//instance.commChan <- &types.PbftMessage{Payload: &types.PbftMessage_Prepare{Prepare: prep}} /// use channel to send prepare to ProtocolManager. Zhiguo
		return nil
	}

	return nil
}

func (instance *pbftCore) recvPrepare(prep *types.Prepare) error {
	logger.Debugf("Replica %d received prepare from replica %d for view=%d/seqNo=%d",
		instance.id, prep.ReplicaId, prep.View, prep.SequenceNumber)

	if instance.primary(prep.View) == prep.ReplicaId {
		//	logger.Warningf("Replica %d received prepare from primary, ignoring", instance.id)
		return nil
	}

	//=>TODO. --Agzs
	if !instance.inWV(prep.View, prep.SequenceNumber) {
		if prep.SequenceNumber != instance.h && !instance.skipInProgress {
			//	logger.Warningf("Replica %d ignoring prepare for view=%d/seqNo=%d: not in-wv, in view %d, low water mark %d", instance.id, prep.View, prep.SequenceNumber, instance.view, instance.h)
		} else {
			// This is perfectly normal
			//	logger.Debugf("Replica %d ignoring prepare for view=%d/seqNo=%d: not in-wv, in view %d, low water mark %d", instance.id, prep.View, prep.SequenceNumber, instance.view, instance.h)
		}
		return nil
	}

	cert := instance.getCert(prep.View, prep.SequenceNumber)

	for _, prevPrep := range cert.prepare {
		if prevPrep.ReplicaId == prep.ReplicaId {
			//	logger.Warningf("Ignoring duplicate prepare from %d", prep.ReplicaId)
			return nil
		}
	}
	cert.prepare = append(cert.prepare, prep)
	instance.persistPSet() //=> --Agzs

	return instance.maybeSendCommit(string(prep.BlockHash), prep.View, prep.SequenceNumber)
}

func (instance *pbftCore) maybeSendCommit(digest string, v uint64, n uint64) error {
	cert := instance.getCert(v, n)
	if instance.prepared(digest, v, n) && !cert.sentCommit {
		logger.Debugf("Replica %d broadcasting commit for view=%d/seqNo=%d",
			instance.id, v, n)
		commit := &types.Commit{
			View:           v,
			SequenceNumber: n,
			BlockHash:      unsafe.StringBytes(digest),
			ReplicaId:      instance.id,
		}
		cert.sentCommit = true
		instance.recvCommit(commit)

		msg := &types.PbftMessage{
			// PrePrepare: nil,
			// Prepare:    nil,
			// Commit:     commit,
			// Checkpoint: nil,
			// ViewChange: nil,
			// NewView:    nil,
			//FetchBlockMsg: nil,
			Sender:      instance.id,
			PayloadCode: types.CommitMsg,
			Payload:     commit,
		} //=>change --Agzs
		instance.innerBroadcast(msg)
		//instance.commChan <- &types.PbftMessage{Payload: &types.PbftMessage_Commit{Commit: commit}} /// use channel to send commit to ProtocolManager. Zhiguo
		///return instance.innerBroadcast(&Message{&Message_Commit{commit}})
	}
	return nil
}

func (instance *pbftCore) recvCommit(commit *types.Commit) error {
	logger.Debugf("Replica %d received commit from replica %d for view=%d/seqNo=%d",
		instance.id, commit.ReplicaId, commit.View, commit.SequenceNumber)

	//=>TODO. --Agzs
	if !instance.inWV(commit.View, commit.SequenceNumber) {
		if commit.SequenceNumber != instance.h && !instance.skipInProgress {
			//	logger.Warningf("Replica %d ignoring commit for view=%d/seqNo=%d: not in-wv, in view %d, low water mark %d", instance.id, commit.View, commit.SequenceNumber, instance.view, instance.h)
		} else {
			// This is perfectly normal
			//	logger.Debugf("Replica %d ignoring commit for view=%d/seqNo=%d: not in-wv, in view %d, low water mark %d", instance.id, commit.View, commit.SequenceNumber, instance.view, instance.h)
		}
		return nil
	}

	cert := instance.getCert(commit.View, commit.SequenceNumber)
	for _, prevCommit := range cert.commit {
		if prevCommit.ReplicaId == commit.ReplicaId {
			//	logger.Warningf("Ignoring duplicate commit from %d", commit.ReplicaId)
			return nil
		}
	}
	cert.commit = append(cert.commit, commit)

	if instance.committed(string(commit.BlockHash), commit.View, commit.SequenceNumber) {
		instance.stopTimer()
		instance.lastNewViewTimeout = instance.newViewTimeout
		delete(instance.outstandingBlocks, string(commit.BlockHash))

		////////////--xiaobei 11.23
		// instance.execDoneSync()
		// //log.Info("execDoneSync() end") //=>test. --Agzs
		// if instance.skipInProgress {
		// 	instance.retryStateTransfer(nil)
		// 	log.Info("retryStateTransfer() end") //=>test. --Agzs
		// }
		// instance.processNewView() ////--xiaobei 11.23
		////////////

		instance.executeOutstanding() ////xiaobei
		//log.Info("recvCommit(1) --executeOutstanding()") //=>test. --Agzs
		//============================
		//instance.helper.manager.Queue() <- execDoneEvent{} ////xiaobei
		// instance.execDoneSync()
		// //log.Info("execDoneSync() end") //=>test. --Agzs
		// if instance.skipInProgress {
		// 	instance.retryStateTransfer(nil)
		// 	log.Info("retryStateTransfer() end") //=>test. --Agzs
		// }
		//=========================
		//log.Info("recvCommit(2) --send execDoneEvent") //=>test. --Agzs

		// if instance.primary(instance.view) == instance.id { ////--xiaobei 11.23
		// 	instance.finishedChan <- struct{}{} /// inform PBFT consensus is reached.  --Zhiguo
		// }

		//instance.LastExec = commit.SequenceNumber //=> add --Agzs
		//log.Info("recvCommit(3) --finished")           //=>test. --Agzs

		//=>instance.moveStore(commit.BlockHash) //=> delete some certStore and blockStore. --Agzs

		if commit.SequenceNumber == instance.viewChangeSeqNo {
			//	logger.Infof("Replica %d cycling view for seqNo=%d", instance.id, commit.SequenceNumber)
			instance.sendViewChange()
		}
	}

	return nil
}

//=>=====================================================
//=>those following funcs may exist logical errors --Agzs
//=>======================================================

//=> TODO. update highStateTarget to target. --Agzs
func (instance *pbftCore) updateHighStateTarget(target *stateUpdateTarget) {
	if instance.highStateTarget != nil && instance.highStateTarget.seqNo >= target.seqNo {
		//	logger.Debugf("Replica %d not updating state target to seqNo %d, has target for seqNo %d", instance.id, target.seqNo, instance.highStateTarget.seqNo)
		return
	}

	instance.highStateTarget = target
}

//=>TODO. change the state, like skipInProgress、Helper.valid  --Agzs
func (instance *pbftCore) stateTransfer(optional *stateUpdateTarget) {
	if !instance.skipInProgress {
		//	logger.Debugf("Replica %d is out of sync, pending state transfer", instance.id)
		instance.skipInProgress = true
		instance.helper.InvalidateState()
	}

	instance.retryStateTransfer(optional)
}

//=>TODO. call skipTo() to create stateUpdateEvent which processed in processEvent(). --Agzs
//// xiaobei
func (instance *pbftCore) retryStateTransfer(optional *stateUpdateTarget) {
	if instance.currentExec != nil {
		//	logger.Debugf("Replica %d is currently mid-execution, it must wait for the execution to complete before performing state transfer", instance.id)
		return
	}

	if instance.stateTransferring {
		//	logger.Debugf("Replica %d is currently mid state transfer, it must wait for this state transfer to complete before initiating a new one", instance.id)
		return
	}

	target := optional
	if target == nil {
		if instance.highStateTarget == nil {
			//	logger.Debugf("Replica %d has no targets to attempt state transfer to, delaying", instance.id)
			return
		}
		target = instance.highStateTarget
	}

	instance.stateTransferring = true

	//logger.Debugf("Replica %d is initiating state transfer to seqNo %d", instance.id, target.seqNo)
	instance.helper.skipTo(target.seqNo, target.id, target.replicas)

}

// func (instance *pbftCore) skipTo(seqNo uint64, id []byte, replicas []uint64) {
// 	info := &types.BlockchainInfo{} ////pb.BlockchainInfo{}->types.BlockchainInfo{} --xiaobei
// 	err := proto.Unmarshal(id, info)
// 	if err != nil {
// 		logger.Error(fmt.Sprintf("Error unmarshaling: %s", err))
// 		return
// 	}
// 	instance.UpdateState(&types.checkpointMessage{seqNo, id}, info, getValidatorHandles(replicas)) ////xiaobei
// }
////xiaobei
func (instance *pbftCore) executeOutstanding() {
	if instance.currentExec != nil {
		//logger.Debugf("Replica %d not attempting to executeOutstanding because it is currently executing %d", instance.id, *instance.currentExec)
		return
	}
	//logger.Debugf("Replica %d attempting to executeOutstanding", instance.id)

	for idx := range instance.certStore {
		if instance.executeOne(idx) {
			break
		}
	}

	//logger.Debugf("Replica %d certstore %+v", instance.id, instance.certStore)

	instance.startTimerIfOutstandingBlocks() ////startTimerIfOutstandingRequests->startTimerIfOutstandingBlocks. --xiaobei
}

//=>TODO. judge msgID in certStore is executed or not. --Agzs
////xiaobei
func (instance *pbftCore) executeOne(idx msgID) bool {
	cert, ok := instance.certStore[idx]
	if !ok {
		logger.Infof("----can't get cert") ////xiaobei --12.28
	}
	//logger.Infof("-----*instance.LastExec+1= %d", *instance.LastExec+1) ////xiaobei 1.3
	if idx.n != *instance.LastExec+1 || cert == nil || cert.prePrepare == nil {
		//logger.Infof("-------executeOne err") ////xiaobei 1.3
		return false
	}

	if instance.skipInProgress {
		//	logger.Debugf("Replica %d currently picking a starting point to resume, will not execute", instance.id)
		return false
	}

	// we now have the right sequence number that doesn't create holes

	digest := cert.digest
	//=> block := instance.blockStore[digest] TODO. --Agzs

	// block, ok := instance.blockStore[digest]
	// if !ok {
	// 	logger.Infof("----get block err!!")
	// }
	block := cert.prePrepare.Block ////xiaobei --12.28
	//logger.Infof("-----get block is %+v", block) ////xiaobei --12.28

	if !instance.committed(digest, idx.v, idx.n) {
		//	logger.Infof("-----block is not committed.") ////xiaobei --12.28
		return false
	}

	// we have a commit certificate for this request batch
	currentExec := idx.n
	instance.currentExec = &currentExec

	// null request
	if digest == "" {
		logger.Infof("Replica %d executing/committing null block for view=%d/seqNo=%d",
			instance.id, idx.v, idx.n)
		instance.execDoneSync()
	} else {
		logger.Infof("Replica %d executing/committing block for view=%d/seqNo=%d and digest %x",
			instance.id, idx.v, idx.n, common.StringToHash(digest)) //=>change digest --Agzs
		// synchronously execute, it is the other side's responsibility to execute in the background if needed
		//=>instance.helper.execute(idx.n, block) //TODO. not need to be executed. --Agzs

		//instance.helper.manager.Queue() <- execDoneEvent{}

		////xiaobei --12.27
		if instance.primary(instance.view) == instance.id { ////--xiaobei 11.23
			instance.finishedChan <- struct{}{} /// inform PBFT consensus is reached.  --Zhiguo
		} else {
			logger.Infof("---not primary")
			CommittedBlock <- block
			logger.Infof("---block put in the CommittedBlock")
		}
		////

		//events.SendEvent(instance, execDoneEvent{}) ////xiaobei 12.19

	}
	return true
}

//=> TODO. send a pbftMessage to commChan, then pm read it and send it to pm's Queue(), processed in processEvent() finally.  --Agzs
////initialize checkpoint message and broadcast. --xiaobei
func (instance *pbftCore) Checkpoint(seqNo uint64, id []byte) {
	if seqNo%instance.K != 0 {
		logger.Errorf("Attempted to checkpoint a sequence number (%d) which is not a multiple of the checkpoint interval (%d)", seqNo, instance.K)
		return
	}

	idAsString := base64.StdEncoding.EncodeToString(id)

	logger.Debugf("Replica %d preparing checkpoint for view=%d/seqNo=%d and b64 id of %s",
		instance.id, instance.view, seqNo, idAsString)

	chkpt := &types.Checkpoint{
		SequenceNumber: seqNo,
		ReplicaId:      instance.id,
		Id:             idAsString,
	}
	instance.chkpts[seqNo] = idAsString

	instance.persistCheckpoint(seqNo, id) //=> --Agzs
	instance.recvCheckpoint(chkpt)

	msg := &types.PbftMessage{
		// PrePrepare: nil,
		// Prepare:    nil,
		// Commit:     nil,
		// Checkpoint: chkpt,
		// ViewChange: nil,
		// NewView:    nil,
		//FetchBlockMsg: nil,
		Sender:      instance.id,
		PayloadCode: types.CheckpointMsg,
		Payload:     chkpt,
	}
	instance.innerBroadcast(msg) ////--xiaobei
	///	instance.innerBroadcast(&Message{Payload: &Message_Checkpoint{Checkpoint: chkpt}})
}

////xiaobei
func (instance *pbftCore) execDoneSync() {
	//log.Info("execDoneSync()") //=>test. --Agzs
	if instance.currentExec != nil {
		//	logger.Infof("Replica %d finished execution %d, trying next", instance.id, *instance.currentExec)
		instance.LastExec = instance.currentExec
		if *instance.LastExec%instance.K == 0 {
			instance.Checkpoint(*instance.LastExec, instance.helper.getState())
		}

	} else {
		// XXX This masks a bug, this should not be called when currentExec is nil
		//	logger.Warningf("Replica %d had execDoneSync called, flagging ourselves as out of date", instance.id)
		instance.skipInProgress = true
	}
	instance.currentExec = nil

	instance.executeOutstanding()
}

//=> moveStore delete certStore and certStore every certStorePeriod blocks.
//=> delete certStorePeriod(default 100) blocks per 2*certStorePeriod blocks. --Agzs
// func (instance *pbftCore) moveStore(blockHash []byte) {

// 	if len(instance.certStore) <= certStorePeriod {
// 		return
// 	}
// 	count := 0
// 	number := instance.blockStore[string(blockHash)].Header().Number.Uint64()
// 	for idx, cert := range instance.certStore {
// 		header := cert.prePrepare.Block.Header()
// 		if header.Number.Uint64() < number {
// 			logger.Debugf("Replica %d cleaning quorum certificate for view=%d/seqNo=%d",
// 				instance.id, idx.v, idx.n)
// 			delete(instance.blockStore, cert.digest)
// 			delete(instance.certStore, idx)
// 			count++
// 		}
// 		if 2*count > certStorePeriod {
// 			break
// 		}
// 	}

// }

//=>TODO --Agzs
func (instance *pbftCore) moveWatermarks(n uint64) {
	// round down n to previous low watermark
	h := n / instance.K * instance.K

	for idx, cert := range instance.certStore {
		if idx.n <= h {
			logger.Debugf("Replica %d cleaning quorum certificate for view=%d/seqNo=%d",
				instance.id, idx.v, idx.n)
			instance.persistDelRequestBlock(cert.digest) //=> --Agzs
			delete(instance.blockStore, cert.digest)
			delete(instance.certStore, idx)
		}
	}

	for testChkpt := range instance.checkpointStore {
		if testChkpt.SequenceNumber <= h {
			logger.Debugf("Replica %d cleaning checkpoint message from replica %d, seqNo %d, b64 snapshot id %s",
				instance.id, testChkpt.ReplicaId, testChkpt.SequenceNumber, testChkpt.Id)
			delete(instance.checkpointStore, testChkpt)
		}
	}

	for n := range instance.pset {
		if n <= h {
			delete(instance.pset, n)
		}
	}

	for idx := range instance.qset {
		if idx.n <= h {
			delete(instance.qset, idx)
		}
	}

	for n := range instance.chkpts {
		if n < h {
			delete(instance.chkpts, n)
			instance.persistDelCheckpoint(n) //=> --Agzs
		}
	}

	instance.h = h

	logger.Debugf("Replica %d updated low watermark to %d",
		instance.id, instance.h)

	instance.resubmitBlockMsges()
}

////if checkpoint.seqNo reached  or exceeded H,store it in the instance.hChkpts
////if chkpt.SequenceNumber<high watermark or len(instance.hChkpts) < instance.f+1,reture false. --xiaobei
func (instance *pbftCore) weakCheckpointSetOutOfRange(chkpt *types.Checkpoint) bool {
	H := instance.h + instance.L

	// Track the last observed checkpoint sequence number if it exceeds our high watermark, keyed by replica to prevent unbounded growth
	if chkpt.SequenceNumber < H { ////chkpt.SequenceNumber not the highest sqeNo --xiaobei
		// For non-byzantine nodes, the checkpoint sequence number increases monotonically
		delete(instance.hChkpts, chkpt.ReplicaId)
	} else {
		// We do not track the highest one, as a byzantine node could pick an arbitrarilly high sequence number
		// and even if it recovered to be non-byzantine, we would still believe it to be far ahead
		instance.hChkpts[chkpt.ReplicaId] = chkpt.SequenceNumber

		// If f+1 other replicas have reported checkpoints that were (at one time) outside our watermarks
		// we need to check to see if we have fallen behind.
		if len(instance.hChkpts) >= instance.f+1 {
			chkptSeqNumArray := make([]uint64, len(instance.hChkpts))
			index := 0
			for replicaID, hChkpt := range instance.hChkpts {
				chkptSeqNumArray[index] = hChkpt
				index++
				if hChkpt < H {
					delete(instance.hChkpts, replicaID)
				}
			}
			sort.Sort(sortableUint64Slice(chkptSeqNumArray))

			// If f+1 nodes have issued checkpoints above our high water mark, then
			// we will never record 2f+1 checkpoints for that sequence number, we are out of date
			// (This is because all_replicas - missed - me = 3f+1 - f - 1 = 2f)
			if m := chkptSeqNumArray[len(chkptSeqNumArray)-(instance.f+1)]; m > H {
				logger.Warningf("Replica %d is out of date, f+1 nodes agree checkpoint with seqNo %d exists but our high water mark is %d", instance.id, chkpt.SequenceNumber, H)
				instance.blockStore = make(map[string]*types.Block) // Discard all our requests, as we will never know which were executed, to be addressed in #394
				instance.persistDelAllRequestBlockes()              ////TODO
				instance.moveWatermarks(m)
				instance.outstandingBlocks = make(map[string]*types.Block) ////RequestBatch->Block --xiaobei
				instance.skipInProgress = true                             ////pick a new starting point --xiaobei
				instance.helper.InvalidateState()                          ////instance.consumer.invalidateState() --xiaobei
				instance.stopTimer()

				// TODO, reprocess the already gathered checkpoints, this will make recovery faster, though it is presently correct

				return true
			}
		}
	}

	return false
}

// //InvalidateState is invoked to tell us that consensus realizes the ledger is out of sync
// func (instance *pbftCore) invalidateState() { ////change *helper->*pbftCore --xiaobei
// 	logger.Debug("Invalidating the current state")
// 	instance.valid = false
// }

func (instance *pbftCore) witnessCheckpointWeakCert(chkpt *types.Checkpoint) {
	checkpointMembers := make([]uint64, instance.f+1) // Only ever invoked for the first weak cert, so guaranteed to be f+1
	i := 0
	for testChkpt := range instance.checkpointStore {
		if testChkpt.SequenceNumber == chkpt.SequenceNumber && testChkpt.Id == chkpt.Id && i < instance.f+1 { ////xiaobei --12.27
			checkpointMembers[i] = testChkpt.ReplicaId
			logger.Debugf("Replica %d adding replica %d (handle %v) to weak cert", instance.id, testChkpt.ReplicaId, checkpointMembers[i])
			i++
		}
	}

	snapshotID, err := base64.StdEncoding.DecodeString(chkpt.Id)
	if nil != err {
		err = fmt.Errorf("Replica %d received a weak checkpoint cert which could not be decoded (%s)", instance.id, chkpt.Id)
		logger.Error(err.Error())
		return
	}

	target := &stateUpdateTarget{
		checkpointMessage: checkpointMessage{
			seqNo: chkpt.SequenceNumber,
			id:    snapshotID,
		},
		replicas: checkpointMembers,
	}
	instance.updateHighStateTarget(target)

	if instance.skipInProgress {
		logger.Debugf("Replica %d is catching up and witnessed a weak certificate for checkpoint %d, weak cert attested to by %d of %d (%v)",
			instance.id, chkpt.SequenceNumber, i, instance.replicaCount, checkpointMembers)
		// The view should not be set to active, this should be handled by the yet unimplemented SUSPECT, see https://github.com/hyperledger/fabric/issues/1120
		instance.retryStateTransfer(target)
	}
}

func (instance *pbftCore) recvCheckpoint(chkpt *types.Checkpoint) events.Event {
	logger.Debugf("Replica %d received checkpoint from replica %d, seqNo %d, digest %s",
		instance.id, chkpt.ReplicaId, chkpt.SequenceNumber, chkpt.Id)

	if instance.weakCheckpointSetOutOfRange(chkpt) { ////judge chkpt.sqeNo Whether it exceeds the high watermark,if not,return false. or chkpt.sqeNo exceeds the high watermark but len(instance.hChkpts)< instance.f+1, return false--xiaobei
		return nil
	}

	if !instance.inW(chkpt.SequenceNumber) {
		if chkpt.SequenceNumber != instance.h && !instance.skipInProgress {
			// It is perfectly normal that we receive checkpoints for the watermark we just raised, as we raise it after 2f+1, leaving f replies left
			logger.Warningf("Checkpoint sequence number outside watermarks: seqNo %d, low-mark %d", chkpt.SequenceNumber, instance.h)
		} else {
			logger.Debugf("Checkpoint sequence number outside watermarks: seqNo %d, low-mark %d", chkpt.SequenceNumber, instance.h)
		}
		return nil
	}

	instance.checkpointStore[*chkpt] = true

	// Track how many different checkpoint values we have for the seqNo in question
	diffValues := make(map[string]struct{}) ////seqNo same,but ID different from chkpt.Id
	diffValues[chkpt.Id] = struct{}{}
	matching := 0 ////Track how many same checkpoint values we have for the seqNo in question. --xiaobei
	for testChkpt := range instance.checkpointStore {
		if testChkpt.SequenceNumber == chkpt.SequenceNumber {
			if testChkpt.Id == chkpt.Id {
				matching++
			} else {
				if _, ok := diffValues[testChkpt.Id]; !ok {
					diffValues[testChkpt.Id] = struct{}{}
				}
			}
		}
	}
	logger.Debugf("Replica %d found %d matching checkpoints for seqNo %d, digest %s",
		instance.id, matching, chkpt.SequenceNumber, chkpt.Id)

	// If f+2 different values have been observed, we'll never be able to get a stable cert for this seqNo
	if count := len(diffValues); count > instance.f+1 {
		logger.Panicf("Network unable to find stable certificate for seqNo %d (%d different values observed already)",
			chkpt.SequenceNumber, count)
	}

	if matching == instance.f+1 {
		// We have a weak cert
		// If we have generated a checkpoint for this seqNo, make sure we have a match
		if ownChkptID, ok := instance.chkpts[chkpt.SequenceNumber]; ok {
			if ownChkptID != chkpt.Id {
				logger.Panicf("Own checkpoint for seqNo %d (%s) different from weak checkpoint certificate (%s)",
					chkpt.SequenceNumber, ownChkptID, chkpt.Id)
			}
		}
		instance.witnessCheckpointWeakCert(chkpt)
	}

	if matching < instance.intersectionQuorum() {
		// We do not have a quorum yet
		return nil
	}

	// It is actually just fine if we do not have this checkpoint
	// and should not trigger a state transfer
	// Imagine we are executing sequence number k-1 and we are slow for some reason
	// then everyone else finishes executing k, and we receive a checkpoint quorum
	// which we will agree with very shortly, but do not move our watermarks until
	// we have reached this checkpoint
	// Note, this is not divergent from the paper, as the paper requires that
	// the quorum certificate must contain 2f+1 messages, including its own
	if _, ok := instance.chkpts[chkpt.SequenceNumber]; !ok {
		logger.Debugf("Replica %d found checkpoint quorum for seqNo %d, digest %s, but it has not reached this checkpoint itself yet",
			instance.id, chkpt.SequenceNumber, chkpt.Id)
		if instance.skipInProgress {
			logSafetyBound := instance.h + instance.L/2
			// As an optimization, if we are more than half way out of our log and in state transfer, move our watermarks so we don't lose track of the network
			// if needed, state transfer will restart on completion to a more recent point in time
			if chkpt.SequenceNumber >= logSafetyBound {
				logger.Debugf("Replica %d is in state transfer, but, the network seems to be moving on past %d, moving our watermarks to stay with it", instance.id, logSafetyBound)
				instance.moveWatermarks(chkpt.SequenceNumber)
			}
		}
		return nil
	}

	logger.Debugf("Replica %d found checkpoint quorum for seqNo %d, digest %s",
		instance.id, chkpt.SequenceNumber, chkpt.Id)

	instance.moveWatermarks(chkpt.SequenceNumber)

	return instance.processNewView()
}

// used in view-change to fetch missing assigned, non-checkpointed requests
//=> TODO. fetchRequestBatches -> fetchBlockMsges.
//=> called by processNewView(), used for process newReqBatchMissing.
//=> we plan to achieve it later. --Agzs
func (instance *pbftCore) fetchBlockMsges() (err error) {
	// var msg *types.PbftMessage
	// for digest := range instance.missingReqBatches {
	// 	msg = &types.PbftMessage{Payload: &types.PbftMessage_FetchBlockMsg{FetchBlockMsg: &types.FetchBlockMsg{
	// 		BlockHash: digest,
	// 		ReplicaId: instance.id,
	// 	}}}
	// 	instance.innerBroadcast(msg)
	// }

	return
}

// func (instance *pbftCore) recvFetchBlockMsg(fr *FetchBlockMsg) (err error) {
// 	digest := fr.BlockMsg
// 	if _, ok := instance.blockStore[digest]; !ok {
// 		return nil // we don't have it either
// 	}

// 	block := instance.blockStore[digest]
// 	msg := &PbftMessage{Payload: &PbftMessage_ReturnBlockMsg{ReturnBlockMsg: block}}
// 	msgPacked, err := proto.Marshal(msg)
// 	if err != nil {
// 		return fmt.Errorf("Error marshalling return-request-batch message: %v", err)
// 	}

// 	receiver := fr.ReplicaId
// 	err = instance.consumer.unicast(msgPacked, receiver)

// 	return
// }
//=>TODO. recvReturnRequestBatch -> recvReturnBlock for blockStore. --Agzs
func (instance *pbftCore) recvReturnBlock(block *types.Block) events.Event {
	digest := block.Hash().Str() //=> --Agzs
	if _, ok := instance.missingReqBatches[digest]; !ok {
		return nil // either the wrong digest, or we got it already from someone else
	}
	instance.blockStore[digest] = block
	delete(instance.missingReqBatches, digest)
	instance.persistRequestBlock(digest) //=> --Agzs
	return instance.processNewView()
}

// =============================================================================
// Misc. methods go here
// =============================================================================

// // Marshals a PbftMessage and hands it to the Stack. If toSelf is true,
// // the message is also dispatched to the local instance's RecvMsgSync.
// func (instance *pbftCore) innerBroadcast(msg *types.PbftMessage) error {
// 	msgRaw, err := proto.Marshal(msg)
// 	if err != nil {
// 		return fmt.Errorf("Cannot marshal message %s", err)
// 	}

// 	doByzantine := false
// 	if instance.byzantine {
// 		rand1 := rand.New(rand.NewSource(time.Now().UnixNano()))
// 		doIt := rand1.Intn(3) // go byzantine about 1/3 of the time
// 		if doIt == 1 {
// 			doByzantine = true
// 		}
// 	}

// 	// testing byzantine fault.
// 	if doByzantine {
// 		rand2 := rand.New(rand.NewSource(time.Now().UnixNano()))
// 		ignoreidx := rand2.Intn(instance.N)
// 		for i := 0; i < instance.N; i++ {
// 			if i != ignoreidx && uint64(i) != instance.id { //Pick a random replica and do not send message
// 				instance.consumer.unicast(msgRaw, uint64(i))
// 			} else {
// 				logger.Debugf("PBFT byzantine: not broadcasting to replica %v", i)
// 			}
// 		}
// 	} else {
// 		instance.consumer.broadcast(msgRaw)
// 	}
// 	return nil
// }
//=> overwrite innerBroadcast to simulate Byzantine and send msg to commChan.
//=> restore innerBroadcast in viewchange.go.--Agzs
func (instance *pbftCore) innerBroadcast(msg *types.PbftMessage) error {

	doByzantine := false
	if instance.byzantine {
		rand1 := rand.New(rand.NewSource(time.Now().UnixNano()))
		doIt := rand1.Intn(3) // go byzantine about 1/3 of the time
		if doIt == 1 {
			doByzantine = true
		}
	}

	// testing byzantine fault.
	if doByzantine {
		rand2 := rand.New(rand.NewSource(time.Now().UnixNano()))
		ignoreidx := rand2.Intn(instance.N)
		for i := 0; i < instance.N; i++ {
			if i != ignoreidx && uint64(i) != instance.id { //Pick a random replica and do not send message
				continue
			} else {
				logger.Debugf("PBFT byzantine: not broadcasting to replica %v", i)
			}
		}
	} else {
		instance.commChan <- msg
	}
	return nil
}

func (instance *pbftCore) updateViewChangeSeqNo() {
	if instance.viewChangePeriod <= 0 {
		return
	}
	// Ensure the view change always occurs at a checkpoint boundary
	instance.viewChangeSeqNo = instance.seqNo + instance.viewChangePeriod*instance.K - instance.seqNo%instance.K
	//=> viewChangeSeqNo =「instance.seqNo / instance.K」 * instance.seqNo + instance.viewChangePeriod*instance.K --Agzs
	logger.Debugf("Replica %d updating view change sequence number to %d", instance.id, instance.viewChangeSeqNo)
}

////xiaobei
func (instance *pbftCore) startTimerIfOutstandingBlocks() {
	if instance.skipInProgress || instance.currentExec != nil {
		// Do not start the view change timer if we are executing or state transferring, these take arbitrarilly long amounts of time
		return
	}

	if len(instance.outstandingBlocks) > 0 {
		getOutstandingDigests := func() []string {
			var digests []string
			for digest := range instance.outstandingBlocks {
				digests = append(digests, digest)
			}
			return digests
		}()
		instance.softStartTimer(instance.requestTimeout, fmt.Sprintf("outstanding blocks %v", getOutstandingDigests))
	} else if instance.nullRequestTimeout > 0 {
		timeout := instance.nullRequestTimeout
		if instance.primary(instance.view) != instance.id {
			// we're waiting for the primary to deliver a null request - give it a bit more time
			timeout += instance.requestTimeout
		}
		instance.nullRequestTimer.Reset(timeout, nullRequestEvent{})
	}
}

func (instance *pbftCore) softStartTimer(timeout time.Duration, reason string) {
	//	logger.Debugf("Replica %d soft starting new view timer for %s: %s", instance.id, timeout, reason)
	instance.newViewTimerReason = reason
	instance.timerActive = true
	instance.newViewTimer.SoftReset(timeout, viewChangeTimerEvent{})
}

func (instance *pbftCore) startTimer(timeout time.Duration, reason string) {
	//	logger.Debugf("Replica %d starting new view timer for %s: %s", instance.id, timeout, reason)
	instance.timerActive = true
	instance.newViewTimer.Reset(timeout, viewChangeTimerEvent{})
}

func (instance *pbftCore) stopTimer() {
	//	logger.Debugf("Replica %d stopping a running new view timer", instance.id)
	instance.timerActive = false
	instance.newViewTimer.Stop()
}
