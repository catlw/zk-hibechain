// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package miner

import (
	"bufio"
	"bytes"
	"fmt"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc" ////xiaobei 12.25
	"github.com/ethereum/go-ethereum/consensus/pbft"
	"github.com/ethereum/go-ethereum/consensus/util/events" ////xiaobei 1.2
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"gopkg.in/fatih/set.v0"
)

const (
	resultQueueSize  = 10
	miningLogAtDepth = 5
	HEADS            = 1
)

// Agent can register themself with the worker
type Agent interface {
	Work() chan<- *Work
	SetReturnCh(chan<- *Result)
	Stop()
	Start()
	GetHashRate() int64
}

// Work is the workers current environment and holds
// all of the current state information
type Work struct {
	config *params.ChainConfig
	signer types.Signer

	state     *state.StateDB // apply state changes here
	ancestors *set.Set       // ancestor set (used for checking uncle parent validity)
	family    *set.Set       // family set (used for checking uncle invalidity)
	uncles    *set.Set       // uncle set
	tcount    int            // tx count in cycle
	failedTxs types.Transactions

	Block *types.Block // the new block

	header    *types.Header
	txs       []*types.Transaction
	receipts  []*types.Receipt
	zkfunds   []common.Hash
	createdAt time.Time
}

type Result struct {
	Work  *Work
	Block *types.Block
}

// worker is the main object which takes care of applying messages to the new state
type worker struct {
	headers     []*common.Hash
	headersLock sync.Mutex

	config *params.ChainConfig
	engine consensus.Engine

	mu sync.Mutex

	// update loop
	mux    *event.TypeMux
	events *event.TypeMuxSubscription
	wg     sync.WaitGroup

	agents map[Agent]struct{}
	recv   chan *Result

	eth     Backend
	chain   *core.BlockChain
	proc    core.Validator
	chainDb ethdb.Database

	coinbase common.Address
	extra    []byte

	currentMu sync.Mutex
	current   *Work

	uncleMu        sync.Mutex
	possibleUncles map[common.Hash]*types.Block

	txQueueMu sync.Mutex
	txQueue   map[common.Hash]*types.Transaction

	unconfirmed *unconfirmedBlocks // set of locally mined blocks pending canonicalness confirmations

	// atomic status counters
	mining int32
	atWork int32

	addressIndex   uint32
	fullValidation bool
}

func newWorker(config *params.ChainConfig, engine consensus.Engine, coinbase common.Address, eth Backend, mux *event.TypeMux) *worker {
	worker := &worker{
		headers:        make([]*common.Hash, 0, HEADS),
		config:         config,
		engine:         engine,
		eth:            eth,
		mux:            mux,
		chainDb:        eth.ChainDb(),
		recv:           make(chan *Result, resultQueueSize),
		chain:          eth.BlockChain(),
		proc:           eth.BlockChain().Validator(),
		possibleUncles: make(map[common.Hash]*types.Block),
		coinbase:       coinbase,
		txQueue:        make(map[common.Hash]*types.Transaction),
		agents:         make(map[Agent]struct{}),
		unconfirmed:    newUnconfirmedBlocks(eth.BlockChain(), 5),
		fullValidation: false,
	}
	worker.events = worker.mux.Subscribe(core.ChainHeadEvent{}, core.ChainSideEvent{}, core.TxPreEvent{})
	go worker.update()

	go worker.wait()
	worker.commitNewWork()

	return worker
}

func (self *worker) addHeader(hash *common.Hash) *types.Transaction {

	self.headersLock.Lock()
	defer self.headersLock.Unlock()
	var tx *types.Transaction
	if len(self.headers) == HEADS {
		fmt.Println("add header error,no space")
		return nil
	} else {
		self.headers = append(self.headers, hash)
		if len(self.headers) == HEADS {
			hibeaddr := &ethapi.DHibeAddress{node.ID, self.addressIndex}
			self.addressIndex += 1
			address := hibeaddr.Address()
			db := self.chainDb
			var nonce uint64
			nonceBytes, err := db.Get(append(address.Bytes(), core.AddressNonceSuffix...))
			if err != nil {
				nonce = 0
			} else {
				reader := bytes.NewReader(nonceBytes)
				if err = rlp.Decode(reader, &nonce); err != nil {
					return nil
				}

			}

			node.NewHeaderTime = time.Now()
			tx = types.NewHeaderTransaction(nonce, self.headers, &address, node.LocalLevel)
			nonce += 1
			nonceBytes, err = rlp.EncodeToBytes(nonce)
			if err != nil {
				fmt.Println("EncodeToBytes(nonce) error")
			}

			db.Put(append(address.Bytes(), core.AddressNonceSuffix...), nonceBytes)

			types.DHibeSignTx(tx)
			//fmt.Println(tx)
			//	sig := hibe.Sign(hibe.PrivateKey, hibe.MasterPubKey, types.HibeHash(tx).Bytes(), hibe.Random)
			//	tx.SetDhibeSig(&sig)
			self.headers = self.headers[:0]
		}
	}
	return tx
}

func (self *worker) setEtherbase(addr common.Address) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.coinbase = addr
}

func (self *worker) setExtra(extra []byte) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.extra = extra
}

func (self *worker) pending() (*types.Block, *state.StateDB) {
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	if atomic.LoadInt32(&self.mining) == 0 {
		return types.NewBlock(
			self.current.header,
			self.current.txs,
			nil,
			self.current.receipts,
		), self.current.state.Copy()
	}
	return self.current.Block, self.current.state.Copy()
}

func (self *worker) pendingBlock() *types.Block {
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	if atomic.LoadInt32(&self.mining) == 0 {
		return types.NewBlock(
			self.current.header,
			self.current.txs,
			nil,
			self.current.receipts,
		)
	}
	return self.current.Block
}

func (self *worker) start() {
	self.mu.Lock()
	defer self.mu.Unlock()

	atomic.StoreInt32(&self.mining, 1)

	// spin up agents
	for agent := range self.agents {
		agent.Start()
	}
}

func (self *worker) stop() {
	self.wg.Wait()

	self.mu.Lock()
	defer self.mu.Unlock()
	//log.Info("-----stop the agent") ////xiaobei --12.28
	if atomic.LoadInt32(&self.mining) == 1 {
		for agent := range self.agents {
			//log.Info("----agent.stop()") ////xiaobei --12.28
			agent.Stop()
		}
	}
	atomic.StoreInt32(&self.mining, 0)
	atomic.StoreInt32(&self.atWork, 0)
}

func (self *worker) register(agent Agent) {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.agents[agent] = struct{}{}
	agent.SetReturnCh(self.recv)
}

func (self *worker) unregister(agent Agent) {
	self.mu.Lock()
	defer self.mu.Unlock()
	delete(self.agents, agent)
	agent.Stop()
}

func (self *worker) update() {
	for event := range self.events.Chan() {
		// A real event arrived, process interesting content
		switch ev := event.Data.(type) {
		case core.ChainHeadEvent:
			log.Info("-----------ChainHeadEvent is called!!!")
			time.Sleep(10 * time.Millisecond)
			self.commitNewWork()
		case core.ChainSideEvent:
			self.uncleMu.Lock()
			self.possibleUncles[ev.Block.Hash()] = ev.Block
			self.uncleMu.Unlock()
		case core.TxPreEvent:
			// Apply transaction to the pending state if we're not mining
			if atomic.LoadInt32(&self.mining) == 0 {
				self.currentMu.Lock()

				var signer types.Signer
				if ev.Tx.TxType() == types.TxNormal {
					signer = self.current.signer
				} else if ev.Tx.TxType() == types.TxDhibe || ev.Tx.TxType() == types.TxHeader || ev.Tx.TxType() == types.TxCrossChain {
					signer = types.NewDHibeSigner()
				} else if ev.Tx.TxType() == types.TxZK {
					signer = types.NewZKSigner()
				}
				acc, _ := types.Sender(signer, ev.Tx)
				txs := map[common.Address]types.Transactions{acc: {ev.Tx}}
				txset := types.NewTransactionsByPriceAndNonce(txs)

				self.current.commitTransactions(self.mux, txset, self.chain, self.coinbase)
				self.currentMu.Unlock()
			}
		}
	}
}

func (self *worker) wait() {
	for {
		mustCommitNewWork := true
		for result := range self.recv {

			atomic.AddInt32(&self.atWork, -1)

			if result == nil {
				continue
			}
			block := result.Block
			work := result.Work
			core.CMTFD = append(core.CMTFD, block.Header().ZKFunds...)
			if node.ResultFile != nil {
				wt := bufio.NewWriter(node.ResultFile)
				str := fmt.Sprintf(" block %d consensus confirm time is :%v:\n", block.Number(), time.Now())
				_, err := wt.WriteString(str)
				if err != nil {
					log.Error("write error")
				}
				wt.Flush()
			}
			if self.fullValidation {
				fmt.Println("full validation: block number:", block.Number().Uint64())
				if _, err := self.chain.InsertChain(types.Blocks{block}); err != nil {

					log.Error("Mined invalid block", "err", err)
					continue
				}

				go self.mux.Post(core.NewMinedBlockEvent{Block: block})
			} else {
				fmt.Println("worker receive mined block: ", block.Number().Uint64())
				work.state.CommitTo(self.chainDb, self.config.IsEIP158(block.Number()))
				stat, err := self.chain.WriteBlock(block)
				if err != nil {
					log.Error("Failed writing block to chain", "err", err)
					continue
				}
				if node.ID != node.ROOTID {
					//if true {
					header := block.Hash()
					fmt.Println("generating new header", header.Hex())
					tx := self.addHeader(&header)
					if tx != nil {
						fmt.Println("generated new header", header.Hex())
						go self.mux.Post(core.HeaderTxEvent{Tx: tx})
					}
				}
				// update block hash since it is now available and not when the receipt/log of individual transactions were created
				for _, r := range work.receipts {
					for _, l := range r.Logs {
						l.BlockHash = block.Hash()
					}
				}
				for _, log := range work.state.Logs() {
					log.BlockHash = block.Hash()
				}

				// check if canon block and write transactions
				if stat == core.CanonStatTy {
					// This puts transactions in a extra db for rpc
					core.WriteTransactions(self.chainDb, block)
					// store the receipts
					core.WriteReceipts(self.chainDb, work.receipts)
					// Write map map bloom filters
					core.WriteMipmapBloom(self.chainDb, block.NumberU64(), work.receipts)
					// implicit by posting ChainHeadEvent
					mustCommitNewWork = false
				}

				if consensus.PBFTEngineFlag { //=> --Agzs 18.03.28
					////
					//events.SendEvent(pbft.PBFTCore, pbft.ExecDoneEvent{}) ////xiaobei --12.25
					//eth.ProtocolManager.pbftmanager.Queue() <- CommittedEvent{}
					events.ManagerImpl.Queue() <- pbft.CommittedEvent{} ////xiaobei --1.2
					//log.Info("-----events.ManagerImpl.Queue() <- pbft.CommittedEvent{}")
					////
				}

				// broadcast before waiting for validation
				go func(block *types.Block, logs []*types.Log, receipts []*types.Receipt) {
					self.mux.Post(core.NewMinedBlockEvent{Block: block})
					self.mux.Post(core.ChainEvent{Block: block, Hash: block.Hash(), Logs: logs})

					if stat == core.CanonStatTy {
						self.mux.Post(core.ChainHeadEvent{Block: block})
						log.Info("-------commitnewwork post ChainHeadEvent")
						self.mux.Post(logs)
					}
					if err := core.WriteBlockReceipts(self.chainDb, block.Hash(), block.NumberU64(), receipts); err != nil {
						log.Warn("Failed writing block receipts", "err", err)
					}
				}(block, work.state.Logs(), work.receipts)
			}
			// Insert the block into the set of pending ones to wait for confirmations
			self.unconfirmed.Insert(block.NumberU64(), block.Hash())
			if node.ResultFile == nil {
				filename := fmt.Sprintf("result%d", node.NodeIndex)
				var err error
				node.ResultFile, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
				if err != nil {
					fmt.Println("Open file error")
				}
			}
			if node.ResultFile != nil {
				wt := bufio.NewWriter(node.ResultFile)
				str := fmt.Sprintf(" block %d  written time is :%v:\n", block.Number(), time.Now())
				_, err := wt.WriteString(str)
				if err != nil {
					log.Error("write error")
				}
				wt.Flush()
			}
			if mustCommitNewWork {
				self.commitNewWork()
			}
			if node.ID == node.ROOTID {
				self.mux.Post(core.RootChainBlock{Block: block})
			}
		}

	}
}

// push sends a new work task to currently live miner agents.
func (self *worker) push(work *Work) {
	//log.Info("------push work----") ////xiaobei --12.28
	if atomic.LoadInt32(&self.mining) != 1 {
		return
	}
	for agent := range self.agents {
		atomic.AddInt32(&self.atWork, 1)
		if ch := agent.Work(); ch != nil {
			//log.Info("-----work put in the chan") ////xiaobei --12.28
			ch <- work
		}
	}
}

// makeCurrent creates a new environment for the current cycle.
func (self *worker) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := self.chain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	work := &Work{
		config:    self.config,
		signer:    types.NewEIP155Signer(self.config.ChainId),
		state:     state,
		ancestors: set.New(),
		family:    set.New(),
		uncles:    set.New(),
		header:    header,
		createdAt: time.Now(),
	}

	// when 08 is processed ancestors contain 07 (quick block)
	for _, ancestor := range self.chain.GetBlocksFromHash(parent.Hash(), 7) {
		for _, uncle := range ancestor.Uncles() {
			work.family.Add(uncle.Hash())
		}
		work.family.Add(ancestor.Hash())
		work.ancestors.Add(ancestor.Hash())
	}
	wallets := self.eth.AccountManager().Wallets()
	accounts := make([]accounts.Account, 0, len(wallets))
	for _, wallet := range wallets {
		accounts = append(accounts, wallet.Accounts()...)
	}
	// Keep track of transactions which return errors so they can be removed
	work.tcount = 0
	self.current = work
	return nil
}

func (self *worker) commitNewWork() {
	self.mu.Lock()
	defer self.mu.Unlock()
	self.uncleMu.Lock()
	defer self.uncleMu.Unlock()
	self.currentMu.Lock()
	defer self.currentMu.Unlock()

	tstart := time.Now()
	parent := self.chain.CurrentBlock()

	tstamp := tstart.Unix()
	if parent.Time().Cmp(new(big.Int).SetInt64(tstamp)) >= 0 {
		tstamp = parent.Time().Int64() + 1
	}
	// this will ensure we're not going off too far in the future
	if now := time.Now().Unix(); tstamp > now+1 {
		wait := time.Duration(tstamp-now) * time.Second
		log.Info("Mining too far in the future", "wait", common.PrettyDuration(wait))
		time.Sleep(wait)
	}

	num := parent.Number()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, common.Big1),
		GasLimit:   core.CalcGasLimit(parent),
		GasUsed:    new(big.Int),
		Extra:      self.extra,
		Time:       big.NewInt(tstamp),
	}
	fmt.Println("commit new work, block number: ", header.Number.Uint64())
	fmt.Println("parent header hash", parent.Hash().Hex())
	if node.ResultFile != nil {
		wt := bufio.NewWriter(node.ResultFile)
		str := fmt.Sprintf(" block %d  prepare time is :%v:\n", header.Number, time.Now())
		_, err := wt.WriteString(str)
		if err != nil {
			log.Error("write error")
		}
		wt.Flush()
	}
	// Only set the coinbase if we are mining (avoid spurious block rewards)
	if atomic.LoadInt32(&self.mining) == 1 {
		// do not set coinbase, that is, the block has no author
		// header.Coinbase = self.coinbase
	}
	if err := self.engine.Prepare(self.chain, header); err != nil {
		log.Error("Failed to prepare header for mining", "err", err)
		return
	}
	// If we are care about TheDAO hard-fork check whether to override the extra-data or not
	if daoBlock := self.config.DAOForkBlock; daoBlock != nil {
		// Check whether the block is among the fork extra-override range
		limit := new(big.Int).Add(daoBlock, params.DAOForkExtraRange)
		if header.Number.Cmp(daoBlock) >= 0 && header.Number.Cmp(limit) < 0 {
			// Depending whether we support or oppose the fork, override differently
			if self.config.DAOForkSupport {
				header.Extra = common.CopyBytes(params.DAOForkBlockExtra)
			} else if bytes.Equal(header.Extra, params.DAOForkBlockExtra) {
				header.Extra = []byte{} // If miner opposes, don't let it use the reserved extra-data
			}
		}
	}
	// Could potentially happen if starting to mine in an odd state.
	err := self.makeCurrent(parent, header)
	if err != nil {
		log.Error("Failed to create mining context", "err", err)
		return
	}
	// Create the current work task and check any fork transitions needed
	work := self.current
	if self.config.DAOForkSupport && self.config.DAOForkBlock != nil && self.config.DAOForkBlock.Cmp(header.Number) == 0 {
		misc.ApplyDAOHardFork(work.state)
	}
	pending, err := self.eth.TxPool().Pending()
	//	if len(pending) == 0 {
	//		return
	//	}
	if err != nil {
		fmt.Printf("-----Failed to fetch pending transactions", err)
		log.Error("Failed to fetch pending transactions", "err", err)
		return
	} else {
		for _, tx := range pending {
			fmt.Printf("pending tx is %+v\n", tx)
		}
	}
	txs := types.NewTransactionsByPriceAndNonce(pending)
	work.commitTransactions(self.mux, txs, self.chain, self.coinbase)

	if node.ResultFile != nil {
		wt := bufio.NewWriter(node.ResultFile)
		str := fmt.Sprintf(" block %d  after prepare time is :%v:\n", header.Number, time.Now())
		_, err := wt.WriteString(str)
		if err != nil {
			log.Error("write error")
		}
		wt.Flush()
	}
	self.eth.TxPool().RemoveBatch(work.failedTxs)

	// compute uncles for the new block.
	var (
		uncles    []*types.Header
		badUncles []common.Hash
	)
	for hash, uncle := range self.possibleUncles {
		if len(uncles) == 2 {
			break
		}
		if err := self.commitUncle(work, uncle.Header()); err != nil {
			log.Trace("Bad uncle found and will be removed", "hash", hash)
			log.Trace(fmt.Sprint(uncle))

			badUncles = append(badUncles, hash)
		} else {
			log.Debug("Committing new uncle to block", "hash", hash)
			uncles = append(uncles, uncle.Header())
		}
	}
	for _, hash := range badUncles {
		delete(self.possibleUncles, hash)
	}
	// Create the new block to seal with the consensus engine
	if work.Block, err = self.engine.Finalize(self.chain, header, work.state, work.txs, uncles, work.receipts, work.zkfunds); err != nil {
		log.Error("Failed to finalize block for sealing", "err", err)
		return
	}
	// We only care about logging if we're actually mining.
	if atomic.LoadInt32(&self.mining) == 1 {
		log.Info("Commit new mining work", "number", work.Block.Number(), "txs", work.tcount, "uncles", len(uncles), "elapsed", common.PrettyDuration(time.Since(tstart)))
		self.unconfirmed.Shift(work.Block.NumberU64() - 1)
	}
	self.push(work)
}

func (self *worker) commitUncle(work *Work, uncle *types.Header) error {
	hash := uncle.Hash()
	if work.uncles.Has(hash) {
		return fmt.Errorf("uncle not unique")
	}
	if !work.ancestors.Has(uncle.ParentHash) {
		return fmt.Errorf("uncle's parent unknown (%x)", uncle.ParentHash[0:4])
	}
	if work.family.Has(hash) {
		return fmt.Errorf("uncle already in family (%x)", hash)
	}
	work.uncles.Add(uncle.Hash())
	return nil
}

func (env *Work) commitTransactions(mux *event.TypeMux, txs *types.TransactionsByPriceAndNonce, bc *core.BlockChain, coinbase common.Address) {
	gp := new(core.GasPool).AddGas(env.header.GasLimit)

	var coalescedLogs []*types.Log
	for {
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}
		// Error may be ignored here. The error has already been checked
		// during transaction acceptance is the transaction pool.
		//
		// We use the eip155 signer regardless of the current hf.
		var signer types.Signer
		switch tx.TxType() {
		case types.TxNormal:
			signer = env.signer
		case types.TxDhibe, types.TxHeader, types.TxCrossChain:
			signer = types.NewDHibeSigner()
		case types.TxZK:
			signer = types.NewZKSigner()

		}
		from, _ := types.Sender(signer, tx)
		// Check whether the tx is replay protected. If we're not in the EIP155 hf
		// phase, start ignoring the sender until we do.
		if tx.TxType() == types.TxNormal && tx.Protected() && !env.config.IsEIP155(env.header.Number) {
			log.Trace("Ignoring reply protected transaction", "hash", tx.Hash(), "eip155", env.config.EIP155Block)
			txs.Pop()
			continue
		}
		// Start executing the transaction
		env.state.Prepare(tx.Hash(), common.Hash{}, env.tcount)

		err, logs := env.commitTransaction(tx, bc, coinbase, gp)
		switch err {
		case core.ErrGasLimitReached:
			// Pop the current out-of-gas transaction without shifting in the next from the account
			log.Info("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case nil:
			// Everything ok, collect the logs and shift in the next transaction from the same account
			coalescedLogs = append(coalescedLogs, logs...)
			env.tcount++
			txs.Shift()

		default:
			// Pop the current failed transaction without shifting in the next from the account
			log.Info("Transaction failed, will be removed", "hash", tx.Hash(), "err", err)
			env.failedTxs = append(env.failedTxs, tx)
			txs.Pop()
		}
	}

	if len(coalescedLogs) > 0 || env.tcount > 0 {
		// make a copy, the state caches the logs and these logs get "upgraded" from pending to mined
		// logs by filling in the block hash when the block was mined by the local miner. This can
		// cause a race condition if a log was "upgraded" before the PendingLogsEvent is processed.
		cpy := make([]*types.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		go func(logs []*types.Log, tcount int) {
			if len(logs) > 0 {
				mux.Post(core.PendingLogsEvent{Logs: logs})
			}
			if tcount > 0 {
				mux.Post(core.PendingStateEvent{})
			}
		}(cpy, env.tcount)
	}
}

func (env *Work) commitTransaction(tx *types.Transaction, bc *core.BlockChain, coinbase common.Address, gp *core.GasPool) (error, []*types.Log) {
	snap := env.state.Snapshot()

	receipt, _, err := core.ApplyTransaction(env.config, bc, &coinbase, gp, env.state, env.header, tx, env.header.GasUsed, vm.Config{})
	if err != nil {
		fmt.Printf("------commitTransaction meet err", err)
		env.state.RevertToSnapshot(snap)
		return err, nil
	}
	env.txs = append(env.txs, tx)
	env.receipts = append(env.receipts, receipt)
	if tx.TxCode() == types.TxDeposit {
		env.zkfunds = append(env.zkfunds, tx.ZKCMTfd())
	}
	return nil, receipt.Logs
}
