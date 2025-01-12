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

package eth

import (
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/pbft"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/peerapi"
	"github.com/ethereum/go-ethereum/rlp"

	"gopkg.in/fatih/set.v0"
)

var (
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
)

const (
	maxKnownTxs      = 32768 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks   = 1024  // Maximum block hashes to keep in the known list (prevent DOS)
	handshakeTimeout = 5 * time.Second
	//=> add maxKnownMsg --Agzs
	maxKnownMsgs = 32768 // Maximum pbftMsgs hashes to keep in the known list (prevent DOS)
	//=> add maxKnownAddPeerMsgs --Agzs 11.15
	maxKnownAddPeerMsgs    = 1024 // Maximum addPeerMsgs hashes to keep in the known list (prevent DOS)
	maxKnownRemovePeerMsgs = 1024 // Maximum removePeerMsgs hashes to keep in the known list (prevent DOS)

)

// PeerInfo represents a short summary of the Ethereum sub-protocol metadata known
// about a connected peer.
type PeerInfo struct {
	Version    int      `json:"version"`    // Ethereum protocol version negotiated
	Difficulty *big.Int `json:"difficulty"` // Total difficulty of the peer's blockchain
	Head       string   `json:"head"`       // SHA3 hash of the peer's best owned block
}

type peer struct {
	id       string
	peerFlag uint64 //=>mark peer --Agzs 12.6
	Index    uint32 //index of node in a blockchain

	*p2p.Peer
	rw p2p.MsgReadWriter

	version  int         // Protocol version negotiated
	forkDrop *time.Timer // Timed connection dropper if forks aren't validated in time

	head common.Hash
	td   *big.Int
	lock sync.RWMutex

	knownTxs    *set.Set // Set of transaction hashes known to be known by this peer
	knownBlocks *set.Set // Set of block hashes known to be known by this peer
	//=> add knownMsg --Agzs
	knownMsg *set.Set // Set of pbftMsg hashes known to be known by this peer
	//=> add knownXXXPeerMsg to add or remove peer --Agzs 11.15
	knownAddPeerMsg    *set.Set // Set of addPeerMsg hashes known to be known by this peer
	knownRemovePeerMsg *set.Set // Set of removePeerMsg hashes known to be known by this peer
}

func newPeer(version int, p *p2p.Peer, rw p2p.MsgReadWriter) *peer {
	id := p.ID()

	pe := &peer{
		peerFlag:           p.GetPeerFlag(), //=> add --Agzs 12.6
		Index:              p.GetNodeIndex(),
		Peer:               p,
		rw:                 rw,
		version:            version,
		id:                 fmt.Sprintf("%x", id[:8]),
		knownTxs:           set.New(),
		knownBlocks:        set.New(),
		knownMsg:           set.New(), //=>add knownMsg. --Agzs
		knownAddPeerMsg:    set.New(), //=>add knownAddPeerMsg. --Agzs 11.15
		knownRemovePeerMsg: set.New(), //=>add knownRemovePeerMsg. --Agzs
	}
	fmt.Printf("new peer %d\n", pe.Index)
	return pe
}

////xiaobei 1.9
func newpeerapi(p peerapi.PeerInterface) {
	peerapi.Peer.PEER = p
	return
}

////

// Info gathers and returns a collection of metadata known about a peer.
func (p *peer) Info() *PeerInfo {
	hash, td := p.Head()

	return &PeerInfo{
		Version:    p.version,
		Difficulty: td,
		Head:       hash.Hex(),
	}
}

// Head retrieves a copy of the current head hash and total difficulty of the
// peer.
func (p *peer) Head() (hash common.Hash, td *big.Int) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	copy(hash[:], p.head[:])
	return hash, new(big.Int).Set(p.td)
}

// SetHead updates the head hash and total difficulty of the peer.
func (p *peer) SetHead(hash common.Hash, td *big.Int) {
	p.lock.Lock()
	defer p.lock.Unlock()

	copy(p.head[:], hash[:])
	p.td.Set(td)
}

// MarkBlock marks a block as known for the peer, ensuring that the block will
// never be propagated to this particular peer.
func (p *peer) MarkBlock(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known block hash
	for p.knownBlocks.Size() >= maxKnownBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(hash)
}

// MarkTransaction marks a transaction as known for the peer, ensuring that it
// will never be propagated to this particular peer.
func (p *peer) MarkTransaction(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known transaction hash
	fmt.Println("---------MarkTransaction", hash.Hex())
	for p.knownTxs.Size() >= maxKnownTxs {
		p.knownTxs.Pop()
	}
	p.knownTxs.Add(hash)
}

//=> add MarkMsg() for knownMsg
// MarkMsg marks a pbftMsg as known for the peer, ensuring that the pbftMsg will
// never be propagated to this particular peer.
func (p *peer) MarkMsg(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known block hash
	for p.knownMsg.Size() >= maxKnownMsgs {
		p.knownMsg.Pop()
	}
	p.knownMsg.Add(hash)
}

//=> add MarkAddPeerMsg() for knownAddPeerMsg --Agzs 11.15
// MarkAddPeerMsg marks a addPeerMsg as known for the peer, ensuring that the addPeerMsg will
// never be propagated to this particular peer.
func (p *peer) MarkAddPeerMsg(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known block hash
	for p.knownAddPeerMsg.Size() >= maxKnownAddPeerMsgs {
		p.knownAddPeerMsg.Pop()
	}
	p.knownAddPeerMsg.Add(hash)
}

//=> add MarkRemovePeerMsg() for knownRemovePeerMsg
// MarkRemovePeerMsg marks a removePeerMsg as known for the peer, ensuring that the removePeerMsg will
// never be propagated to this particular peer.
func (p *peer) MarkRemovePeerMsg(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known block hash
	for p.knownRemovePeerMsg.Size() >= maxKnownRemovePeerMsgs {
		p.knownRemovePeerMsg.Pop()
	}
	p.knownRemovePeerMsg.Add(hash)
}

// SendTransactions sends transactions to the peer and includes the hashes
// in its transaction hash set for future reference.
func (p *peer) SendTransactions(txs types.Transactions) error {
	for _, tx := range txs {
		p.knownTxs.Add(tx.Hash())
	}
	return p2p.Send(p.rw, TxMsg, txs)
}

// SendNewBlockHashes announces the availability of a number of blocks through
// a hash notification.
func (p *peer) SendNewBlockHashes(hashes []common.Hash, numbers []uint64) error {
	for _, hash := range hashes {
		p.knownBlocks.Add(hash)
	}
	request := make(newBlockHashesData, len(hashes))
	for i := 0; i < len(hashes); i++ {
		request[i].Hash = hashes[i]
		request[i].Number = numbers[i]
	}
	return p2p.Send(p.rw, NewBlockHashesMsg, request)
}

// SendNewBlock propagates an entire block to a remote peer.
func (p *peer) SendNewBlock(block *types.Block, td *big.Int) error {
	p.knownBlocks.Add(block.Hash())
	return p2p.Send(p.rw, NewBlockMsg, []interface{}{block, td})
}

/////////////////////////////////////
/// for PBFT consensus messages. --Zhiguo 04/10
/// Used by BroadcastMsg of ProtocolManager.
/// TODO: message are encoded using RLP, may not use Protobuf
func (p *peer) SendMsg(msg *types.PbftMessage) error {
	//log.Info("peer.SendMsg() start", "pbftMessageType", reflect.ValueOf(msg.GetPayload()).Type()) //=>test. --Agzs
	p.knownMsg.Add(types.Hash(msg)) //=> add for knowMsg. --Agzs
	//=>return p2p.Send(p.rw, ConsensusMsg, msg)
	//=> add --Agzs
	if x, ok := msg.GetPayload().(*types.PrePrepare); ok {
		return p2p.Send(p.rw, PrePrepareMsg, []interface{}{x, msg.Sender, msg.PayloadCode})
	} else if x, ok := msg.GetPayload().(*types.Prepare); ok {
		return p2p.Send(p.rw, PrepareMsg, []interface{}{x, msg.Sender, msg.PayloadCode})
	} else if x, ok := msg.GetPayload().(*types.Commit); ok {
		return p2p.Send(p.rw, CommitMsg, []interface{}{x, msg.Sender, msg.PayloadCode})
	} else if x, ok := msg.GetPayload().(*types.Checkpoint); ok {
		return p2p.Send(p.rw, CheckpointMsg, []interface{}{x, msg.Sender, msg.PayloadCode})
	} else if x, ok := msg.GetPayload().(*types.ViewChange); ok {
		return p2p.Send(p.rw, ViewChangeMsg, []interface{}{x, msg.Sender, msg.PayloadCode})
	} else if x, ok := msg.GetPayload().(*types.NewView); ok {
		return p2p.Send(p.rw, NewViewMsg, []interface{}{x, msg.Sender, msg.PayloadCode})

	} else if x, ok := msg.GetPayload().(*pbft.TestMsg); ok { //test hibe
		fmt.Println("send PrePrepareTestMsg")
		return p2p.Send(p.rw, PrePrepareTestMsg, []interface{}{x, msg.Sender, msg.PayloadCode})

	} else if x, ok := msg.GetPayload().(*pbft.PrepareTestMsg); ok { //test hibe
		fmt.Println("send PrepareTestMsg")
		return p2p.Send(p.rw, PrepareTestMsg, []interface{}{x, msg.Sender, msg.PayloadCode})

	} else if x, ok := msg.GetPayload().(*pbft.CommitTestMsg); ok { //test hibe
		fmt.Println("send CommitTestMsg")
		return p2p.Send(p.rw, CommitTestMsg, []interface{}{x, msg.Sender, msg.PayloadCode})
	}

	return fmt.Errorf("Invalid message: %v", msg)
}

func (p *peer) SendAddPeerMsg(addPeerMsg *node.URLFlag) error {
	//log.Info("peer.SendAddPeerMsg() start", "url", *addPeerMsg) //=>test. --Agzs
	p.knownAddPeerMsg.Add(types.Hash(addPeerMsg))
	err := p2p.Send(p.rw, AddPeerMsg, addPeerMsg)
	if err != nil {
		return err
	}
	return nil

}

func (p *peer) SendRemovePeerMsg(removePeerMsg *node.URLFlag) error {
	//log.Info("peer.SendRemovePeerMsg() start", "url", *removePeerMsg) //=>test. --Agzs
	p.knownRemovePeerMsg.Add(types.Hash(removePeerMsg))
	err := p2p.Send(p.rw, RemovePeerMsg, removePeerMsg)
	if err != nil {
		return err
	}
	return nil

}

/////////////////////////////////////

// SendBlockHeaders sends a batch of block headers to the remote peer.
func (p *peer) SendBlockHeaders(headers []*types.Header) error {
	return p2p.Send(p.rw, BlockHeadersMsg, headers)
}

// SendBlockBodies sends a batch of block contents to the remote peer.
func (p *peer) SendBlockBodies(bodies []*blockBody) error {
	return p2p.Send(p.rw, BlockBodiesMsg, blockBodiesData(bodies))
}

// SendBlockBodiesRLP sends a batch of block contents to the remote peer from
// an already RLP encoded format.
func (p *peer) SendBlockBodiesRLP(bodies []rlp.RawValue) error {
	return p2p.Send(p.rw, BlockBodiesMsg, bodies)
}

// SendNodeDataRLP sends a batch of arbitrary internal data, corresponding to the
// hashes requested.
func (p *peer) SendNodeData(data [][]byte) error {
	return p2p.Send(p.rw, NodeDataMsg, data)
}

// SendReceiptsRLP sends a batch of transaction receipts, corresponding to the
// ones requested from an already RLP encoded format.
func (p *peer) SendReceiptsRLP(receipts []rlp.RawValue) error {
	return p2p.Send(p.rw, ReceiptsMsg, receipts)
}

// RequestOneHeader is a wrapper around the header query functions to fetch a
// single header. It is used solely by the fetcher.
func (p *peer) RequestOneHeader(hash common.Hash) error {
	p.Log().Debug("Fetching single header", "hash", hash)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: hash}, Amount: uint64(1), Skip: uint64(0), Reverse: false})
}

// RequestHeadersByHash fetches a batch of blocks' headers corresponding to the
// specified header query, based on the hash of an origin block.
func (p *peer) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) error {
	p.Log().Debug("Fetching batch of headers", "count", amount, "fromhash", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestHeadersByNumber fetches a batch of blocks' headers corresponding to the
// specified header query, based on the number of an origin block.
func (p *peer) RequestHeadersByNumber(origin uint64, amount int, skip int, reverse bool) error {
	p.Log().Debug("Fetching batch of headers", "count", amount, "fromnum", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Number: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestBodies fetches a batch of blocks' bodies corresponding to the hashes
// specified.
func (p *peer) RequestBodies(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of block bodies", "count", len(hashes))
	return p2p.Send(p.rw, GetBlockBodiesMsg, hashes)
}

// RequestNodeData fetches a batch of arbitrary data from a node's known state
// data, corresponding to the specified hashes.
func (p *peer) RequestNodeData(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of state data", "count", len(hashes))
	return p2p.Send(p.rw, GetNodeDataMsg, hashes)
}

// RequestReceipts fetches a batch of transaction receipts from a remote node.
func (p *peer) RequestReceipts(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of receipts", "count", len(hashes))
	return p2p.Send(p.rw, GetReceiptsMsg, hashes)
}

// Handshake executes the eth protocol handshake, negotiating version number,
// network IDs, difficulties, head and genesis blocks.
// => add blockchainId --Agzs 12.25
func (p *peer) Handshake(network, blockchainId uint64, td *big.Int, head common.Hash, genesis common.Hash) error {
	// Send out own handshake in a new thread
	errc := make(chan error, 2)
	var status statusData // safe to read after two values have been received from errc

	go func() {
		errc <- p2p.Send(p.rw, StatusMsg, &statusData{
			ProtocolVersion: uint32(p.version),
			NetworkId:       network,
			BlockchainId:    blockchainId, //=>add for blockchainId --Agzs 12.25
			TD:              td,
			CurrentBlock:    head,
			GenesisBlock:    genesis,
		})
	}()
	go func() {
		errc <- p.readStatus(network, blockchainId, &status, genesis) //=>add for blockchainId --Agzs 12.25
	}()
	timeout := time.NewTimer(handshakeTimeout)
	defer timeout.Stop()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errc:
			if err != nil {
				return err
			}
		case <-timeout.C:
			return p2p.DiscReadTimeout
		}
	}
	p.td, p.head = status.TD, status.CurrentBlock
	return nil
}

func (p *peer) readStatus(network, blockchainId uint64, status *statusData, genesis common.Hash) (err error) {
	msg, err := p.rw.ReadMsg()
	//=>log.Info("Read the next message from the remote peer in peer.readStatus()") //=>test. --Agzs
	if err != nil {
		//fmt.Println("---readMsg return err,err is:----", err) ////test--xiaobei 10.31
		return err
	}
	if msg.Code != StatusMsg {
		//fmt.Println("---msg.Code != StatusMsg----") ////test--xiaobei 10.31
		return errResp(ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, StatusMsg)
	}
	if msg.Size > ProtocolMaxMsgSize {
		//fmt.Println("--- msg.Size > ProtocolMaxMsgSize----") ////test--xiaobei 10.31
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	// Decode the handshake and make sure everything matches
	if err := msg.Decode(&status); err != nil {
		return errResp(ErrDecode, "msg %v: %v", msg, err)
	}
	//=>log.Info("peer.readStatus", "status.GenesisBlock", status.GenesisBlock, "genesis", genesis) //=>test. --Agzs
	if status.GenesisBlock != genesis {
		//fmt.Println("--- status.GenesisBlock != genesis----") ////test--xiaobei 10.31
		return errResp(ErrGenesisBlockMismatch, "%x (!= %x)", status.GenesisBlock[:8], genesis[:8])
	}
	if status.NetworkId != network {
		//fmt.Println("--- status.NetworkId != network----") ////test--xiaobei 10.31
		return errResp(ErrNetworkIdMismatch, "%d (!= %d)", status.NetworkId, network)
	}
	//=> add BlockchainId judge --Agzs 12.25
	if status.BlockchainId != blockchainId {
		//fmt.Println("--- status.BlockchainId != blockchainId----") ////test--xiaobei 10.31
		return errResp(ErrBlockchainIdMismatch, "%d (!= %d)", status.BlockchainId, blockchainId)
	}
	//=> end --Agzs
	if int(status.ProtocolVersion) != p.version {
		//fmt.Println("--- int(status.ProtocolVersion) != p.version----") ////test--xiaobei 10.31
		return errResp(ErrProtocolVersionMismatch, "%d (!= %d)", status.ProtocolVersion, p.version)
	}
	//=>log.Info("peer.readStatus() -----------end------------") //=>test. --Agzs
	return nil
}

// String implements fmt.Stringer.
func (p *peer) String() string {
	return fmt.Sprintf("Peer %s [%s]", p.id,
		fmt.Sprintf("eth/%2d", p.version),
	)
}

// peerSet represents the collection of active peers currently participating in
// the Ethereum sub-protocol.
type peerSet struct {
	peers  map[string]*peer
	lock   sync.RWMutex
	closed bool
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[string]*peer),
	}
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known.
func (ps *peerSet) Register(p *peer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errClosed
	}
	if _, ok := ps.peers[p.id]; ok {
		return errAlreadyRegistered
	}
	ps.peers[p.id] = p
	return nil
}

// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *peerSet) Unregister(id string) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if _, ok := ps.peers[id]; !ok {
		return errNotRegistered
	}
	delete(ps.peers, id)
	return nil
}

// Peer retrieves the registered peer with the given id.
func (ps *peerSet) Peer(id string) *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return ps.peers[id]
}

// Len returns if the current number of peers in the set.
func (ps *peerSet) Len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	return len(ps.peers)
}

// PeersWithoutBlock retrieves a list of peers that do not have a given block in
// their set of known hashes.
func (ps *peerSet) PeersWithoutBlock(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownBlocks.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

// PeersWithoutTx retrieves a list of peers that do not have a given transaction
// in their set of known hashes.
func (ps *peerSet) PeersWithoutTx(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownTxs.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

//=> add for KnownMsg. --Agzs
// PeersWithoutMsg retrieves a list of peers that do not have a given pbftMessage
// in their set of known hashes.
func (ps *peerSet) PeersWithoutMsg(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownMsg.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

//=> add for KnownAddPeerMsg. --Agzs
// PeersWithoutAddPeerMsg retrieves a list of peers that do not have a given addPeerMsg
// in their set of known hashes.
func (ps *peerSet) PeersWithoutAddPeerMsg(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownAddPeerMsg.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

//=> add for KnownRemovePeerMsg. --Agzs
// PeersWithoutRemovePeerMsg retrieves a list of peers that do not have a given removePeerMsg
// in their set of known hashes.
func (ps *peerSet) PeersWithoutRemovePeerMsg(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownRemovePeerMsg.Has(hash) {
			list = append(list, p)
		}
	}
	return list
}

// BestPeer retrieves the known peer with the currently highest total difficulty.
func (ps *peerSet) BestPeer() *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var (
		bestPeer *peer
		bestTd   *big.Int
	)
	for _, p := range ps.peers {
		if _, td := p.Head(); bestPeer == nil || td.Cmp(bestTd) > 0 {
			bestPeer, bestTd = p, td
		}
	}
	////xiaobei 1.9
	if bestPeer != nil {
		newpeerapi(bestPeer)
		peerapi.Peer.ID = bestPeer.id
		//fmt.Printf("----best peer id is %s",peerapi.Peer.ID)
	}
	////
	return bestPeer
}

// Close disconnects all peers.
// No new peers can be registered after Close has returned.
func (ps *peerSet) Close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		p.Disconnect(p2p.DiscQuitting)
	}
	ps.closed = true
}
