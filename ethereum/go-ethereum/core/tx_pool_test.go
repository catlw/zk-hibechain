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

package core

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
)

func transaction(nonce uint64, gaslimit *big.Int, key *ecdsa.PrivateKey) *types.Transaction {
	return pricedTransaction(nonce, gaslimit, big.NewInt(1), key)
}

func pricedTransaction(nonce uint64, gaslimit, gasprice *big.Int, key *ecdsa.PrivateKey) *types.Transaction {
	tx, _ := types.SignTx(types.NewTransaction(nonce, common.Address{}, big.NewInt(100), gaslimit, gasprice, nil), types.HomesteadSigner{}, key)
	return tx
}

func setupTxPool() (*TxPool, *ecdsa.PrivateKey) {
	db, _ := ethdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))

	key, _ := crypto.GenerateKey()
	pool := NewTxPool(DefaultTxPoolConfig, params.TestChainConfig, new(event.TypeMux), func() (*state.StateDB, error) { return statedb, nil }, func() *big.Int { return big.NewInt(1000000) })
	pool.resetState()

	return pool, key
}

// validateTxPoolInternals checks various consistency invariants within the pool.
func validateTxPoolInternals(pool *TxPool) error {
	pool.mu.RLock()
	defer pool.mu.RUnlock()

	// Ensure the total transaction set is consistent with pending + queued
	pending, queued := pool.stats()
	if total := len(pool.all); total != pending+queued {
		return fmt.Errorf("total transaction count %d != %d pending + %d queued", total, pending, queued)
	}
	if priced := pool.priced.items.Len() - pool.priced.stales; priced != pending+queued {
		return fmt.Errorf("total priced transaction count %d != %d pending + %d queued", priced, pending, queued)
	}
	// Ensure the next nonce to assign is the correct one
	for addr, txs := range pool.pending {
		// Find the last transaction
		var last uint64
		for nonce, _ := range txs.txs.items {
			if last < nonce {
				last = nonce
			}
		}
		if nonce := pool.pendingState.GetNonce(addr); nonce != last+1 {
			return fmt.Errorf("pending nonce mismatch: have %v, want %v", nonce, last+1)
		}
	}
	return nil
}

func deriveSender(tx *types.Transaction) (common.Address, error) {
	return types.Sender(types.HomesteadSigner{}, tx)
}

// This test simulates a scenario where a new block is imported during a
// state reset and tests whether the pending state is in sync with the
// block head event that initiated the resetState().
func TestStateChangeDuringPoolReset(t *testing.T) {
	var (
		db, _      = ethdb.NewMemDatabase()
		key, _     = crypto.GenerateKey()
		address    = crypto.PubkeyToAddress(key.PublicKey)
		mux        = new(event.TypeMux)
		statedb, _ = state.New(common.Hash{}, state.NewDatabase(db))
		trigger    = false
	)

	// setup pool with 2 transaction in it
	statedb.SetBalance(address, new(big.Int).SetUint64(params.Ether))

	tx0 := transaction(0, big.NewInt(100000), key)
	tx1 := transaction(1, big.NewInt(100000), key)

	// stateFunc is used multiple times to reset the pending state.
	// when simulate is true it will create a state that indicates
	// that tx0 and tx1 are included in the chain.
	stateFunc := func() (*state.StateDB, error) {
		// delay "state change" by one. The tx pool fetches the
		// state multiple times and by delaying it a bit we simulate
		// a state change between those fetches.
		stdb := statedb
		if trigger {
			statedb, _ = state.New(common.Hash{}, state.NewDatabase(db))
			// simulate that the new head block included tx0 and tx1
			statedb.SetNonce(address, 2)
			statedb.SetBalance(address, new(big.Int).SetUint64(params.Ether))
			trigger = false
		}
		return stdb, nil
	}

	gasLimitFunc := func() *big.Int { return big.NewInt(1000000000) }

	pool := NewTxPool(DefaultTxPoolConfig, params.TestChainConfig, mux, stateFunc, gasLimitFunc)
	defer pool.Stop()
	pool.resetState()

	nonce := pool.State().GetNonce(address)
	if nonce != 0 {
		t.Fatalf("Invalid nonce, want 0, got %d", nonce)
	}

	pool.AddRemotes(types.Transactions{tx0, tx1})

	nonce = pool.State().GetNonce(address)
	if nonce != 2 {
		t.Fatalf("Invalid nonce, want 2, got %d", nonce)
	}

	// trigger state change in the background
	trigger = true

	pool.resetState()

	pendingTx, err := pool.Pending()
	if err != nil {
		t.Fatalf("Could not fetch pending transactions: %v", err)
	}

	for addr, txs := range pendingTx {
		t.Logf("%0x: %d\n", addr, len(txs))
	}

	nonce = pool.State().GetNonce(address)
	if nonce != 2 {
		t.Fatalf("Invalid nonce, want 2, got %d", nonce)
	}
}

func TestInvalidTransactions(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	tx := transaction(0, big.NewInt(100), key)
	from, _ := deriveSender(tx)
	currentState, _ := pool.currentState()
	currentState.AddBalance(from, big.NewInt(1))
	if err := pool.AddRemote(tx); err != ErrInsufficientFunds {
		t.Error("expected", ErrInsufficientFunds)
	}

	balance := new(big.Int).Add(tx.Value(), new(big.Int).Mul(tx.Gas(), tx.GasPrice()))
	currentState.AddBalance(from, balance)
	if err := pool.AddRemote(tx); err != ErrIntrinsicGas {
		t.Error("expected", ErrIntrinsicGas, "got", err)
	}

	currentState.SetNonce(from, 1)
	currentState.AddBalance(from, big.NewInt(0xffffffffffffff))
	tx = transaction(0, big.NewInt(100000), key)
	if err := pool.AddRemote(tx); err != ErrNonceTooLow {
		t.Error("expected", ErrNonceTooLow)
	}

	tx = transaction(1, big.NewInt(100000), key)
	pool.gasPrice = big.NewInt(1000)
	if err := pool.AddRemote(tx); err != ErrUnderpriced {
		t.Error("expected", ErrUnderpriced, "got", err)
	}
	if err := pool.AddLocal(tx); err != nil {
		t.Error("expected", nil, "got", err)
	}
}

func TestTransactionQueue(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	tx := transaction(0, big.NewInt(100), key)
	from, _ := deriveSender(tx)
	currentState, _ := pool.currentState()
	currentState.AddBalance(from, big.NewInt(1000))
	pool.resetState()
	pool.enqueueTx(tx.Hash(), tx)

	pool.promoteExecutables(currentState, []common.Address{from})
	if len(pool.pending) != 1 {
		t.Error("expected valid txs to be 1 is", len(pool.pending))
	}

	tx = transaction(1, big.NewInt(100), key)
	from, _ = deriveSender(tx)
	currentState.SetNonce(from, 2)
	pool.enqueueTx(tx.Hash(), tx)
	pool.promoteExecutables(currentState, []common.Address{from})
	if _, ok := pool.pending[from].txs.items[tx.Nonce()]; ok {
		t.Error("expected transaction to be in tx pool")
	}

	if len(pool.queue) > 0 {
		t.Error("expected transaction queue to be empty. is", len(pool.queue))
	}

	pool, key = setupTxPool()
	tx1 := transaction(0, big.NewInt(100), key)
	tx2 := transaction(10, big.NewInt(100), key)
	tx3 := transaction(11, big.NewInt(100), key)
	from, _ = deriveSender(tx1)
	currentState, _ = pool.currentState()
	currentState.AddBalance(from, big.NewInt(1000))
	pool.resetState()

	pool.enqueueTx(tx1.Hash(), tx1)
	pool.enqueueTx(tx2.Hash(), tx2)
	pool.enqueueTx(tx3.Hash(), tx3)

	pool.promoteExecutables(currentState, []common.Address{from})

	if len(pool.pending) != 1 {
		t.Error("expected tx pool to be 1, got", len(pool.pending))
	}
	if pool.queue[from].Len() != 2 {
		t.Error("expected len(queue) == 2, got", pool.queue[from].Len())
	}
}

func TestRemoveTx(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	addr := crypto.PubkeyToAddress(key.PublicKey)
	currentState, _ := pool.currentState()
	currentState.AddBalance(addr, big.NewInt(1))

	tx1 := transaction(0, big.NewInt(100), key)
	tx2 := transaction(2, big.NewInt(100), key)

	pool.promoteTx(addr, tx1.Hash(), tx1)
	pool.enqueueTx(tx2.Hash(), tx2)

	if len(pool.queue) != 1 {
		t.Error("expected queue to be 1, got", len(pool.queue))
	}
	if len(pool.pending) != 1 {
		t.Error("expected pending to be 1, got", len(pool.pending))
	}
	pool.Remove(tx1.Hash())
	pool.Remove(tx2.Hash())

	if len(pool.queue) > 0 {
		t.Error("expected queue to be 0, got", len(pool.queue))
	}
	if len(pool.pending) > 0 {
		t.Error("expected pending to be 0, got", len(pool.pending))
	}
}

func TestNegativeValue(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	tx, _ := types.SignTx(types.NewTransaction(0, common.Address{}, big.NewInt(-1), big.NewInt(100), big.NewInt(1), nil), types.HomesteadSigner{}, key)
	from, _ := deriveSender(tx)
	currentState, _ := pool.currentState()
	currentState.AddBalance(from, big.NewInt(1))
	if err := pool.AddRemote(tx); err != ErrNegativeValue {
		t.Error("expected", ErrNegativeValue, "got", err)
	}
}

func TestTransactionChainFork(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	addr := crypto.PubkeyToAddress(key.PublicKey)
	resetState := func() {
		db, _ := ethdb.NewMemDatabase()
		statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
		pool.currentState = func() (*state.StateDB, error) { return statedb, nil }
		currentState, _ := pool.currentState()
		currentState.AddBalance(addr, big.NewInt(100000000000000))
		pool.resetState()
	}
	resetState()

	tx := transaction(0, big.NewInt(100000), key)
	if _, err := pool.add(tx, false); err != nil {
		t.Error("didn't expect error", err)
	}
	pool.RemoveBatch([]*types.Transaction{tx})

	// reset the pool's internal state
	resetState()
	if _, err := pool.add(tx, false); err != nil {
		t.Error("didn't expect error", err)
	}
}

func TestTransactionDoubleNonce(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	addr := crypto.PubkeyToAddress(key.PublicKey)
	resetState := func() {
		db, _ := ethdb.NewMemDatabase()
		statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
		pool.currentState = func() (*state.StateDB, error) { return statedb, nil }
		currentState, _ := pool.currentState()
		currentState.AddBalance(addr, big.NewInt(100000000000000))
		pool.resetState()
	}
	resetState()

	signer := types.HomesteadSigner{}
	tx1, _ := types.SignTx(types.NewTransaction(0, common.Address{}, big.NewInt(100), big.NewInt(100000), big.NewInt(1), nil), signer, key)
	tx2, _ := types.SignTx(types.NewTransaction(0, common.Address{}, big.NewInt(100), big.NewInt(1000000), big.NewInt(2), nil), signer, key)
	tx3, _ := types.SignTx(types.NewTransaction(0, common.Address{}, big.NewInt(100), big.NewInt(1000000), big.NewInt(1), nil), signer, key)

	// Add the first two transaction, ensure higher priced stays only
	if replace, err := pool.add(tx1, false); err != nil || replace {
		t.Errorf("first transaction insert failed (%v) or reported replacement (%v)", err, replace)
	}
	if replace, err := pool.add(tx2, false); err != nil || !replace {
		t.Errorf("second transaction insert failed (%v) or not reported replacement (%v)", err, replace)
	}
	state, _ := pool.currentState()
	pool.promoteExecutables(state, []common.Address{addr})
	if pool.pending[addr].Len() != 1 {
		t.Error("expected 1 pending transactions, got", pool.pending[addr].Len())
	}
	if tx := pool.pending[addr].txs.items[0]; tx.Hash() != tx2.Hash() {
		t.Errorf("transaction mismatch: have %x, want %x", tx.Hash(), tx2.Hash())
	}
	// Add the third transaction and ensure it's not saved (smaller price)
	pool.add(tx3, false)
	pool.promoteExecutables(state, []common.Address{addr})
	if pool.pending[addr].Len() != 1 {
		t.Error("expected 1 pending transactions, got", pool.pending[addr].Len())
	}
	if tx := pool.pending[addr].txs.items[0]; tx.Hash() != tx2.Hash() {
		t.Errorf("transaction mismatch: have %x, want %x", tx.Hash(), tx2.Hash())
	}
	// Ensure the total transaction count is correct
	if len(pool.all) != 1 {
		t.Error("expected 1 total transactions, got", len(pool.all))
	}
}

func TestMissingNonce(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	addr := crypto.PubkeyToAddress(key.PublicKey)
	currentState, _ := pool.currentState()
	currentState.AddBalance(addr, big.NewInt(100000000000000))
	tx := transaction(1, big.NewInt(100000), key)
	if _, err := pool.add(tx, false); err != nil {
		t.Error("didn't expect error", err)
	}
	if len(pool.pending) != 0 {
		t.Error("expected 0 pending transactions, got", len(pool.pending))
	}
	if pool.queue[addr].Len() != 1 {
		t.Error("expected 1 queued transaction, got", pool.queue[addr].Len())
	}
	if len(pool.all) != 1 {
		t.Error("expected 1 total transactions, got", len(pool.all))
	}
}

func TestNonceRecovery(t *testing.T) {
	const n = 10
	pool, key := setupTxPool()
	defer pool.Stop()

	addr := crypto.PubkeyToAddress(key.PublicKey)
	currentState, _ := pool.currentState()
	currentState.SetNonce(addr, n)
	currentState.AddBalance(addr, big.NewInt(100000000000000))
	pool.resetState()
	tx := transaction(n, big.NewInt(100000), key)
	if err := pool.AddRemote(tx); err != nil {
		t.Error(err)
	}
	// simulate some weird re-order of transactions and missing nonce(s)
	currentState.SetNonce(addr, n-1)
	pool.resetState()
	if fn := pool.pendingState.GetNonce(addr); fn != n+1 {
		t.Errorf("expected nonce to be %d, got %d", n+1, fn)
	}
}

func TestRemovedTxEvent(t *testing.T) {
	pool, key := setupTxPool()
	defer pool.Stop()

	tx := transaction(0, big.NewInt(1000000), key)
	from, _ := deriveSender(tx)
	currentState, _ := pool.currentState()
	currentState.AddBalance(from, big.NewInt(1000000000000))
	pool.resetState()
	pool.eventMux.Post(RemovedTransactionEvent{types.Transactions{tx}})
	pool.eventMux.Post(ChainHeadEvent{nil})
	if pool.pending[from].Len() != 1 {
		t.Error("expected 1 pending tx, got", pool.pending[from].Len())
	}
	if len(pool.all) != 1 {
		t.Error("expected 1 total transactions, got", len(pool.all))
	}
}

// Tests that if an account runs out of funds, any pending and queued transactions
// are dropped.
func TestTransactionDropping(t *testing.T) {
	// Create a test account and fund it
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))

	state, _ := pool.currentState()
	state.AddBalance(account, big.NewInt(1000))

	// Add some pending and some queued transactions
	var (
		tx0  = transaction(0, big.NewInt(100), key)
		tx1  = transaction(1, big.NewInt(200), key)
		tx2  = transaction(2, big.NewInt(300), key)
		tx10 = transaction(10, big.NewInt(100), key)
		tx11 = transaction(11, big.NewInt(200), key)
		tx12 = transaction(12, big.NewInt(300), key)
	)
	pool.promoteTx(account, tx0.Hash(), tx0)
	pool.promoteTx(account, tx1.Hash(), tx1)
	pool.promoteTx(account, tx2.Hash(), tx2)
	pool.enqueueTx(tx10.Hash(), tx10)
	pool.enqueueTx(tx11.Hash(), tx11)
	pool.enqueueTx(tx12.Hash(), tx12)

	// Check that pre and post validations leave the pool as is
	if pool.pending[account].Len() != 3 {
		t.Errorf("pending transaction mismatch: have %d, want %d", pool.pending[account].Len(), 3)
	}
	if pool.queue[account].Len() != 3 {
		t.Errorf("queued transaction mismatch: have %d, want %d", pool.queue[account].Len(), 3)
	}
	if len(pool.all) != 6 {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), 6)
	}
	pool.resetState()
	if pool.pending[account].Len() != 3 {
		t.Errorf("pending transaction mismatch: have %d, want %d", pool.pending[account].Len(), 3)
	}
	if pool.queue[account].Len() != 3 {
		t.Errorf("queued transaction mismatch: have %d, want %d", pool.queue[account].Len(), 3)
	}
	if len(pool.all) != 6 {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), 6)
	}
	// Reduce the balance of the account, and check that invalidated transactions are dropped
	state.AddBalance(account, big.NewInt(-650))
	pool.resetState()

	if _, ok := pool.pending[account].txs.items[tx0.Nonce()]; !ok {
		t.Errorf("funded pending transaction missing: %v", tx0)
	}
	if _, ok := pool.pending[account].txs.items[tx1.Nonce()]; !ok {
		t.Errorf("funded pending transaction missing: %v", tx0)
	}
	if _, ok := pool.pending[account].txs.items[tx2.Nonce()]; ok {
		t.Errorf("out-of-fund pending transaction present: %v", tx1)
	}
	if _, ok := pool.queue[account].txs.items[tx10.Nonce()]; !ok {
		t.Errorf("funded queued transaction missing: %v", tx10)
	}
	if _, ok := pool.queue[account].txs.items[tx11.Nonce()]; !ok {
		t.Errorf("funded queued transaction missing: %v", tx10)
	}
	if _, ok := pool.queue[account].txs.items[tx12.Nonce()]; ok {
		t.Errorf("out-of-fund queued transaction present: %v", tx11)
	}
	if len(pool.all) != 4 {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), 4)
	}
	// Reduce the block gas limit, check that invalidated transactions are dropped
	pool.gasLimit = func() *big.Int { return big.NewInt(100) }
	pool.resetState()

	if _, ok := pool.pending[account].txs.items[tx0.Nonce()]; !ok {
		t.Errorf("funded pending transaction missing: %v", tx0)
	}
	if _, ok := pool.pending[account].txs.items[tx1.Nonce()]; ok {
		t.Errorf("over-gased pending transaction present: %v", tx1)
	}
	if _, ok := pool.queue[account].txs.items[tx10.Nonce()]; !ok {
		t.Errorf("funded queued transaction missing: %v", tx10)
	}
	if _, ok := pool.queue[account].txs.items[tx11.Nonce()]; ok {
		t.Errorf("over-gased queued transaction present: %v", tx11)
	}
	if len(pool.all) != 2 {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), 2)
	}
}

// Tests that if a transaction is dropped from the current pending pool (e.g. out
// of fund), all consecutive (still valid, but not executable) transactions are
// postponed back into the future queue to prevent broadcasting them.
func TestTransactionPostponing(t *testing.T) {
	// Create a test account and fund it
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))

	state, _ := pool.currentState()
	state.AddBalance(account, big.NewInt(1000))

	// Add a batch consecutive pending transactions for validation
	txns := []*types.Transaction{}
	for i := 0; i < 100; i++ {
		var tx *types.Transaction
		if i%2 == 0 {
			tx = transaction(uint64(i), big.NewInt(100), key)
		} else {
			tx = transaction(uint64(i), big.NewInt(500), key)
		}
		pool.promoteTx(account, tx.Hash(), tx)
		txns = append(txns, tx)
	}
	// Check that pre and post validations leave the pool as is
	if pool.pending[account].Len() != len(txns) {
		t.Errorf("pending transaction mismatch: have %d, want %d", pool.pending[account].Len(), len(txns))
	}
	if len(pool.queue) != 0 {
		t.Errorf("queued transaction mismatch: have %d, want %d", pool.queue[account].Len(), 0)
	}
	if len(pool.all) != len(txns) {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), len(txns))
	}
	pool.resetState()
	if pool.pending[account].Len() != len(txns) {
		t.Errorf("pending transaction mismatch: have %d, want %d", pool.pending[account].Len(), len(txns))
	}
	if len(pool.queue) != 0 {
		t.Errorf("queued transaction mismatch: have %d, want %d", pool.queue[account].Len(), 0)
	}
	if len(pool.all) != len(txns) {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), len(txns))
	}
	// Reduce the balance of the account, and check that transactions are reorganised
	state.AddBalance(account, big.NewInt(-750))
	pool.resetState()

	if _, ok := pool.pending[account].txs.items[txns[0].Nonce()]; !ok {
		t.Errorf("tx %d: valid and funded transaction missing from pending pool: %v", 0, txns[0])
	}
	if _, ok := pool.queue[account].txs.items[txns[0].Nonce()]; ok {
		t.Errorf("tx %d: valid and funded transaction present in future queue: %v", 0, txns[0])
	}
	for i, tx := range txns[1:] {
		if i%2 == 1 {
			if _, ok := pool.pending[account].txs.items[tx.Nonce()]; ok {
				t.Errorf("tx %d: valid but future transaction present in pending pool: %v", i+1, tx)
			}
			if _, ok := pool.queue[account].txs.items[tx.Nonce()]; !ok {
				t.Errorf("tx %d: valid but future transaction missing from future queue: %v", i+1, tx)
			}
		} else {
			if _, ok := pool.pending[account].txs.items[tx.Nonce()]; ok {
				t.Errorf("tx %d: out-of-fund transaction present in pending pool: %v", i+1, tx)
			}
			if _, ok := pool.queue[account].txs.items[tx.Nonce()]; ok {
				t.Errorf("tx %d: out-of-fund transaction present in future queue: %v", i+1, tx)
			}
		}
	}
	if len(pool.all) != len(txns)/2 {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), len(txns)/2)
	}
}

// Tests that if the transaction count belonging to a single account goes above
// some threshold, the higher transactions are dropped to prevent DOS attacks.
func TestTransactionQueueAccountLimiting(t *testing.T) {
	// Create a test account and fund it
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))

	state, _ := pool.currentState()
	state.AddBalance(account, big.NewInt(1000000))
	pool.resetState()

	// Keep queuing up transactions and make sure all above a limit are dropped
	for i := uint64(1); i <= DefaultTxPoolConfig.AccountQueue+5; i++ {
		if err := pool.AddRemote(transaction(i, big.NewInt(100000), key)); err != nil {
			t.Fatalf("tx %d: failed to add transaction: %v", i, err)
		}
		if len(pool.pending) != 0 {
			t.Errorf("tx %d: pending pool size mismatch: have %d, want %d", i, len(pool.pending), 0)
		}
		if i <= DefaultTxPoolConfig.AccountQueue {
			if pool.queue[account].Len() != int(i) {
				t.Errorf("tx %d: queue size mismatch: have %d, want %d", i, pool.queue[account].Len(), i)
			}
		} else {
			if pool.queue[account].Len() != int(DefaultTxPoolConfig.AccountQueue) {
				t.Errorf("tx %d: queue limit mismatch: have %d, want %d", i, pool.queue[account].Len(), DefaultTxPoolConfig.AccountQueue)
			}
		}
	}
	if len(pool.all) != int(DefaultTxPoolConfig.AccountQueue) {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), DefaultTxPoolConfig.AccountQueue)
	}
}

// Tests that if the transaction count belonging to multiple accounts go above
// some threshold, the higher transactions are dropped to prevent DOS attacks.
//
// This logic should not hold for local transactions, unless the local tracking
// mechanism is disabled.
func TestTransactionQueueGlobalLimiting(t *testing.T) {
	testTransactionQueueGlobalLimiting(t, false)
}
func TestTransactionQueueGlobalLimitingNoLocals(t *testing.T) {
	testTransactionQueueGlobalLimiting(t, true)
}

func testTransactionQueueGlobalLimiting(t *testing.T, nolocals bool) {
	// Create the pool to test the limit enforcement with
	db, _ := ethdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))

	config := DefaultTxPoolConfig
	config.NoLocals = nolocals
	config.GlobalQueue = config.AccountQueue*3 - 1 // reduce the queue limits to shorten test time (-1 to make it non divisible)

	pool := NewTxPool(config, params.TestChainConfig, new(event.TypeMux), func() (*state.StateDB, error) { return statedb, nil }, func() *big.Int { return big.NewInt(1000000) })
	defer pool.Stop()
	pool.resetState()

	// Create a number of test accounts and fund them (last one will be the local)
	state, _ := pool.currentState()

	keys := make([]*ecdsa.PrivateKey, 5)
	for i := 0; i < len(keys); i++ {
		keys[i], _ = crypto.GenerateKey()
		state.AddBalance(crypto.PubkeyToAddress(keys[i].PublicKey), big.NewInt(1000000))
	}
	local := keys[len(keys)-1]

	// Generate and queue a batch of transactions
	nonces := make(map[common.Address]uint64)

	txs := make(types.Transactions, 0, 3*config.GlobalQueue)
	for len(txs) < cap(txs) {
		key := keys[rand.Intn(len(keys)-1)] // skip adding transactions with the local account
		addr := crypto.PubkeyToAddress(key.PublicKey)

		txs = append(txs, transaction(nonces[addr]+1, big.NewInt(100000), key))
		nonces[addr]++
	}
	// Import the batch and verify that limits have been enforced
	pool.AddRemotes(txs)

	queued := 0
	for addr, list := range pool.queue {
		if list.Len() > int(config.AccountQueue) {
			t.Errorf("addr %x: queued accounts overflown allowance: %d > %d", addr, list.Len(), config.AccountQueue)
		}
		queued += list.Len()
	}
	if queued > int(config.GlobalQueue) {
		t.Fatalf("total transactions overflow allowance: %d > %d", queued, config.GlobalQueue)
	}
	// Generate a batch of transactions from the local account and import them
	txs = txs[:0]
	for i := uint64(0); i < 3*config.GlobalQueue; i++ {
		txs = append(txs, transaction(i+1, big.NewInt(100000), local))
	}
	pool.AddLocals(txs)

	// If locals are disabled, the previous eviction algorithm should apply here too
	if nolocals {
		queued := 0
		for addr, list := range pool.queue {
			if list.Len() > int(config.AccountQueue) {
				t.Errorf("addr %x: queued accounts overflown allowance: %d > %d", addr, list.Len(), config.AccountQueue)
			}
			queued += list.Len()
		}
		if queued > int(config.GlobalQueue) {
			t.Fatalf("total transactions overflow allowance: %d > %d", queued, config.GlobalQueue)
		}
	} else {
		// Local exemptions are enabled, make sure the local account owned the queue
		if len(pool.queue) != 1 {
			t.Errorf("multiple accounts in queue: have %v, want %v", len(pool.queue), 1)
		}
		// Also ensure no local transactions are ever dropped, even if above global limits
		if queued := pool.queue[crypto.PubkeyToAddress(local.PublicKey)].Len(); uint64(queued) != 3*config.GlobalQueue {
			t.Fatalf("local account queued transaction count mismatch: have %v, want %v", queued, 3*config.GlobalQueue)
		}
	}
}

// Tests that if an account remains idle for a prolonged amount of time, any
// non-executable transactions queued up are dropped to prevent wasting resources
// on shuffling them around.
//
// This logic should not hold for local transactions, unless the local tracking
// mechanism is disabled.
func TestTransactionQueueTimeLimiting(t *testing.T)         { testTransactionQueueTimeLimiting(t, false) }
func TestTransactionQueueTimeLimitingNoLocals(t *testing.T) { testTransactionQueueTimeLimiting(t, true) }

func testTransactionQueueTimeLimiting(t *testing.T, nolocals bool) {
	// Reduce the eviction interval to a testable amount
	defer func(old time.Duration) { evictionInterval = old }(evictionInterval)
	evictionInterval = 250 * time.Millisecond

	// Create the pool to test the non-expiration enforcement
	db, _ := ethdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))

	config := DefaultTxPoolConfig
	config.Lifetime = 250 * time.Millisecond
	config.NoLocals = nolocals

	pool := NewTxPool(config, params.TestChainConfig, new(event.TypeMux), func() (*state.StateDB, error) { return statedb, nil }, func() *big.Int { return big.NewInt(1000000) })
	defer pool.Stop()
	pool.resetState()

	// Create two test accounts to ensure remotes expire but locals do not
	local, _ := crypto.GenerateKey()
	remote, _ := crypto.GenerateKey()

	state, _ := pool.currentState()
	state.AddBalance(crypto.PubkeyToAddress(local.PublicKey), big.NewInt(1000000000))
	state.AddBalance(crypto.PubkeyToAddress(remote.PublicKey), big.NewInt(1000000000))

	// Add the two transactions and ensure they both are queued up
	if err := pool.AddLocal(pricedTransaction(1, big.NewInt(100000), big.NewInt(1), local)); err != nil {
		t.Fatalf("failed to add local transaction: %v", err)
	}
	if err := pool.AddRemote(pricedTransaction(1, big.NewInt(100000), big.NewInt(1), remote)); err != nil {
		t.Fatalf("failed to add remote transaction: %v", err)
	}
	pending, queued := pool.stats()
	if pending != 0 {
		t.Fatalf("pending transactions mismatched: have %d, want %d", pending, 0)
	}
	if queued != 2 {
		t.Fatalf("queued transactions mismatched: have %d, want %d", queued, 2)
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
	// Wait a bit for eviction to run and clean up any leftovers, and ensure only the local remains
	time.Sleep(2 * config.Lifetime)

	pending, queued = pool.stats()
	if pending != 0 {
		t.Fatalf("pending transactions mismatched: have %d, want %d", pending, 0)
	}
	if nolocals {
		if queued != 0 {
			t.Fatalf("queued transactions mismatched: have %d, want %d", queued, 0)
		}
	} else {
		if queued != 1 {
			t.Fatalf("queued transactions mismatched: have %d, want %d", queued, 1)
		}
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
}

// Tests that even if the transaction count belonging to a single account goes
// above some threshold, as long as the transactions are executable, they are
// accepted.
func TestTransactionPendingLimiting(t *testing.T) {
	// Create a test account and fund it
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))

	state, _ := pool.currentState()
	state.AddBalance(account, big.NewInt(1000000))
	pool.resetState()

	// Keep queuing up transactions and make sure all above a limit are dropped
	for i := uint64(0); i < DefaultTxPoolConfig.AccountQueue+5; i++ {
		if err := pool.AddRemote(transaction(i, big.NewInt(100000), key)); err != nil {
			t.Fatalf("tx %d: failed to add transaction: %v", i, err)
		}
		if pool.pending[account].Len() != int(i)+1 {
			t.Errorf("tx %d: pending pool size mismatch: have %d, want %d", i, pool.pending[account].Len(), i+1)
		}
		if len(pool.queue) != 0 {
			t.Errorf("tx %d: queue size mismatch: have %d, want %d", i, pool.queue[account].Len(), 0)
		}
	}
	if len(pool.all) != int(DefaultTxPoolConfig.AccountQueue+5) {
		t.Errorf("total transaction mismatch: have %d, want %d", len(pool.all), DefaultTxPoolConfig.AccountQueue+5)
	}
}

// Tests that the transaction limits are enforced the same way irrelevant whether
// the transactions are added one by one or in batches.
func TestTransactionQueueLimitingEquivalency(t *testing.T)   { testTransactionLimitingEquivalency(t, 1) }
func TestTransactionPendingLimitingEquivalency(t *testing.T) { testTransactionLimitingEquivalency(t, 0) }

func testTransactionLimitingEquivalency(t *testing.T, origin uint64) {
	// Add a batch of transactions to a pool one by one
	pool1, key1 := setupTxPool()
	account1, _ := deriveSender(transaction(0, big.NewInt(0), key1))
	state1, _ := pool1.currentState()
	state1.AddBalance(account1, big.NewInt(1000000))

	for i := uint64(0); i < DefaultTxPoolConfig.AccountQueue+5; i++ {
		if err := pool1.AddRemote(transaction(origin+i, big.NewInt(100000), key1)); err != nil {
			t.Fatalf("tx %d: failed to add transaction: %v", i, err)
		}
	}
	// Add a batch of transactions to a pool in one big batch
	pool2, key2 := setupTxPool()
	account2, _ := deriveSender(transaction(0, big.NewInt(0), key2))
	state2, _ := pool2.currentState()
	state2.AddBalance(account2, big.NewInt(1000000))

	txns := []*types.Transaction{}
	for i := uint64(0); i < DefaultTxPoolConfig.AccountQueue+5; i++ {
		txns = append(txns, transaction(origin+i, big.NewInt(100000), key2))
	}
	pool2.AddRemotes(txns)

	// Ensure the batch optimization honors the same pool mechanics
	if len(pool1.pending) != len(pool2.pending) {
		t.Errorf("pending transaction count mismatch: one-by-one algo: %d, batch algo: %d", len(pool1.pending), len(pool2.pending))
	}
	if len(pool1.queue) != len(pool2.queue) {
		t.Errorf("queued transaction count mismatch: one-by-one algo: %d, batch algo: %d", len(pool1.queue), len(pool2.queue))
	}
	if len(pool1.all) != len(pool2.all) {
		t.Errorf("total transaction count mismatch: one-by-one algo %d, batch algo %d", len(pool1.all), len(pool2.all))
	}
	if err := validateTxPoolInternals(pool1); err != nil {
		t.Errorf("pool 1 internal state corrupted: %v", err)
	}
	if err := validateTxPoolInternals(pool2); err != nil {
		t.Errorf("pool 2 internal state corrupted: %v", err)
	}
}

// Tests that if the transaction count belonging to multiple accounts go above
// some hard threshold, the higher transactions are dropped to prevent DOS
// attacks.
func TestTransactionPendingGlobalLimiting(t *testing.T) {
	// Create the pool to test the limit enforcement with
	db, _ := ethdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))

	config := DefaultTxPoolConfig
	config.GlobalSlots = config.AccountSlots * 10

	pool := NewTxPool(config, params.TestChainConfig, new(event.TypeMux), func() (*state.StateDB, error) { return statedb, nil }, func() *big.Int { return big.NewInt(1000000) })
	defer pool.Stop()
	pool.resetState()

	// Create a number of test accounts and fund them
	state, _ := pool.currentState()

	keys := make([]*ecdsa.PrivateKey, 5)
	for i := 0; i < len(keys); i++ {
		keys[i], _ = crypto.GenerateKey()
		state.AddBalance(crypto.PubkeyToAddress(keys[i].PublicKey), big.NewInt(1000000))
	}
	// Generate and queue a batch of transactions
	nonces := make(map[common.Address]uint64)

	txs := types.Transactions{}
	for _, key := range keys {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		for j := 0; j < int(config.GlobalSlots)/len(keys)*2; j++ {
			txs = append(txs, transaction(nonces[addr], big.NewInt(100000), key))
			nonces[addr]++
		}
	}
	// Import the batch and verify that limits have been enforced
	pool.AddRemotes(txs)

	pending := 0
	for _, list := range pool.pending {
		pending += list.Len()
	}
	if pending > int(config.GlobalSlots) {
		t.Fatalf("total pending transactions overflow allowance: %d > %d", pending, config.GlobalSlots)
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
}

// Tests that if transactions start being capped, transasctions are also removed from 'all'
func TestTransactionCapClearsFromAll(t *testing.T) {
	// Create the pool to test the limit enforcement with
	db, _ := ethdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))

	config := DefaultTxPoolConfig
	config.AccountSlots = 2
	config.AccountQueue = 2
	config.GlobalSlots = 8

	pool := NewTxPool(config, params.TestChainConfig, new(event.TypeMux), func() (*state.StateDB, error) { return statedb, nil }, func() *big.Int { return big.NewInt(1000000) })
	defer pool.Stop()
	pool.resetState()

	// Create a number of test accounts and fund them
	state, _ := pool.currentState()

	key, _ := crypto.GenerateKey()
	addr := crypto.PubkeyToAddress(key.PublicKey)
	state.AddBalance(addr, big.NewInt(1000000))

	txs := types.Transactions{}
	for j := 0; j < int(config.GlobalSlots)*2; j++ {
		txs = append(txs, transaction(uint64(j), big.NewInt(100000), key))
	}
	// Import the batch and verify that limits have been enforced
	pool.AddRemotes(txs)
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
}

// Tests that if the transaction count belonging to multiple accounts go above
// some hard threshold, if they are under the minimum guaranteed slot count then
// the transactions are still kept.
func TestTransactionPendingMinimumAllowance(t *testing.T) {
	// Create the pool to test the limit enforcement with
	db, _ := ethdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))

	config := DefaultTxPoolConfig
	config.GlobalSlots = 0

	pool := NewTxPool(config, params.TestChainConfig, new(event.TypeMux), func() (*state.StateDB, error) { return statedb, nil }, func() *big.Int { return big.NewInt(1000000) })
	defer pool.Stop()
	pool.resetState()

	// Create a number of test accounts and fund them
	state, _ := pool.currentState()

	keys := make([]*ecdsa.PrivateKey, 5)
	for i := 0; i < len(keys); i++ {
		keys[i], _ = crypto.GenerateKey()
		state.AddBalance(crypto.PubkeyToAddress(keys[i].PublicKey), big.NewInt(1000000))
	}
	// Generate and queue a batch of transactions
	nonces := make(map[common.Address]uint64)

	txs := types.Transactions{}
	for _, key := range keys {
		addr := crypto.PubkeyToAddress(key.PublicKey)
		for j := 0; j < int(config.AccountSlots)*2; j++ {
			txs = append(txs, transaction(nonces[addr], big.NewInt(100000), key))
			nonces[addr]++
		}
	}
	// Import the batch and verify that limits have been enforced
	pool.AddRemotes(txs)

	for addr, list := range pool.pending {
		if list.Len() != int(config.AccountSlots) {
			t.Errorf("addr %x: total pending transactions mismatch: have %d, want %d", addr, list.Len(), config.AccountSlots)
		}
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
}

// Tests that setting the transaction pool gas price to a higher value correctly
// discards everything cheaper than that and moves any gapped transactions back
// from the pending pool to the queue.
//
// Note, local transactions are never allowed to be dropped.
func TestTransactionPoolRepricing(t *testing.T) {
	// Create the pool to test the pricing enforcement with
	db, _ := ethdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))

	pool := NewTxPool(DefaultTxPoolConfig, params.TestChainConfig, new(event.TypeMux), func() (*state.StateDB, error) { return statedb, nil }, func() *big.Int { return big.NewInt(1000000) })
	defer pool.Stop()
	pool.resetState()

	// Create a number of test accounts and fund them
	state, _ := pool.currentState()

	keys := make([]*ecdsa.PrivateKey, 3)
	for i := 0; i < len(keys); i++ {
		keys[i], _ = crypto.GenerateKey()
		state.AddBalance(crypto.PubkeyToAddress(keys[i].PublicKey), big.NewInt(1000000))
	}
	// Generate and queue a batch of transactions, both pending and queued
	txs := types.Transactions{}

	txs = append(txs, pricedTransaction(0, big.NewInt(100000), big.NewInt(2), keys[0]))
	txs = append(txs, pricedTransaction(1, big.NewInt(100000), big.NewInt(1), keys[0]))
	txs = append(txs, pricedTransaction(2, big.NewInt(100000), big.NewInt(2), keys[0]))

	txs = append(txs, pricedTransaction(1, big.NewInt(100000), big.NewInt(2), keys[1]))
	txs = append(txs, pricedTransaction(2, big.NewInt(100000), big.NewInt(1), keys[1]))
	txs = append(txs, pricedTransaction(3, big.NewInt(100000), big.NewInt(2), keys[1]))

	ltx := pricedTransaction(0, big.NewInt(100000), big.NewInt(1), keys[2])

	// Import the batch and that both pending and queued transactions match up
	pool.AddRemotes(txs)
	pool.AddLocal(ltx)

	pending, queued := pool.stats()
	if pending != 4 {
		t.Fatalf("pending transactions mismatched: have %d, want %d", pending, 4)
	}
	if queued != 3 {
		t.Fatalf("queued transactions mismatched: have %d, want %d", queued, 3)
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
	// Reprice the pool and check that underpriced transactions get dropped
	pool.SetGasPrice(big.NewInt(2))

	pending, queued = pool.stats()
	if pending != 2 {
		t.Fatalf("pending transactions mismatched: have %d, want %d", pending, 2)
	}
	if queued != 3 {
		t.Fatalf("queued transactions mismatched: have %d, want %d", queued, 3)
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
	// Check that we can't add the old transactions back
	if err := pool.AddRemote(pricedTransaction(1, big.NewInt(100000), big.NewInt(1), keys[0])); err != ErrUnderpriced {
		t.Fatalf("adding underpriced pending transaction error mismatch: have %v, want %v", err, ErrUnderpriced)
	}
	if err := pool.AddRemote(pricedTransaction(2, big.NewInt(100000), big.NewInt(1), keys[1])); err != ErrUnderpriced {
		t.Fatalf("adding underpriced queued transaction error mismatch: have %v, want %v", err, ErrUnderpriced)
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
	// However we can add local underpriced transactions
	tx := pricedTransaction(1, big.NewInt(100000), big.NewInt(1), keys[2])
	if err := pool.AddLocal(tx); err != nil {
		t.Fatalf("failed to add underpriced local transaction: %v", err)
	}
	if pending, _ = pool.stats(); pending != 3 {
		t.Fatalf("pending transactions mismatched: have %d, want %d", pending, 3)
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
}

// Tests that when the pool reaches its global transaction limit, underpriced
// transactions are gradually shifted out for more expensive ones and any gapped
// pending transactions are moved into te queue.
//
// Note, local transactions are never allowed to be dropped.
func TestTransactionPoolUnderpricing(t *testing.T) {
	// Create the pool to test the pricing enforcement with
	db, _ := ethdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))

	config := DefaultTxPoolConfig
	config.GlobalSlots = 2
	config.GlobalQueue = 2

	pool := NewTxPool(config, params.TestChainConfig, new(event.TypeMux), func() (*state.StateDB, error) { return statedb, nil }, func() *big.Int { return big.NewInt(1000000) })
	defer pool.Stop()
	pool.resetState()

	// Create a number of test accounts and fund them
	state, _ := pool.currentState()

	keys := make([]*ecdsa.PrivateKey, 3)
	for i := 0; i < len(keys); i++ {
		keys[i], _ = crypto.GenerateKey()
		state.AddBalance(crypto.PubkeyToAddress(keys[i].PublicKey), big.NewInt(1000000))
	}
	// Generate and queue a batch of transactions, both pending and queued
	txs := types.Transactions{}

	txs = append(txs, pricedTransaction(0, big.NewInt(100000), big.NewInt(1), keys[0]))
	txs = append(txs, pricedTransaction(1, big.NewInt(100000), big.NewInt(2), keys[0]))

	txs = append(txs, pricedTransaction(1, big.NewInt(100000), big.NewInt(1), keys[1]))

	ltx := pricedTransaction(0, big.NewInt(100000), big.NewInt(1), keys[2])

	// Import the batch and that both pending and queued transactions match up
	pool.AddRemotes(txs)
	pool.AddLocal(ltx)

	pending, queued := pool.stats()
	if pending != 3 {
		t.Fatalf("pending transactions mismatched: have %d, want %d", pending, 3)
	}
	if queued != 1 {
		t.Fatalf("queued transactions mismatched: have %d, want %d", queued, 1)
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
	// Ensure that adding an underpriced transaction on block limit fails
	if err := pool.AddRemote(pricedTransaction(0, big.NewInt(100000), big.NewInt(1), keys[1])); err != ErrUnderpriced {
		t.Fatalf("adding underpriced pending transaction error mismatch: have %v, want %v", err, ErrUnderpriced)
	}
	// Ensure that adding high priced transactions drops cheap ones, but not own
	if err := pool.AddRemote(pricedTransaction(0, big.NewInt(100000), big.NewInt(3), keys[1])); err != nil {
		t.Fatalf("failed to add well priced transaction: %v", err)
	}
	if err := pool.AddRemote(pricedTransaction(2, big.NewInt(100000), big.NewInt(4), keys[1])); err != nil {
		t.Fatalf("failed to add well priced transaction: %v", err)
	}
	if err := pool.AddRemote(pricedTransaction(3, big.NewInt(100000), big.NewInt(5), keys[1])); err != nil {
		t.Fatalf("failed to add well priced transaction: %v", err)
	}
	pending, queued = pool.stats()
	if pending != 2 {
		t.Fatalf("pending transactions mismatched: have %d, want %d", pending, 2)
	}
	if queued != 2 {
		t.Fatalf("queued transactions mismatched: have %d, want %d", queued, 2)
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
	// Ensure that adding local transactions can push out even higher priced ones
	tx := pricedTransaction(1, big.NewInt(100000), big.NewInt(0), keys[2])
	if err := pool.AddLocal(tx); err != nil {
		t.Fatalf("failed to add underpriced local transaction: %v", err)
	}
	pending, queued = pool.stats()
	if pending != 2 {
		t.Fatalf("pending transactions mismatched: have %d, want %d", pending, 2)
	}
	if queued != 2 {
		t.Fatalf("queued transactions mismatched: have %d, want %d", queued, 2)
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
}

// Tests that the pool rejects replacement transactions that don't meet the minimum
// price bump required.
func TestTransactionReplacement(t *testing.T) {
	// Create the pool to test the pricing enforcement with
	db, _ := ethdb.NewMemDatabase()
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))

	pool := NewTxPool(DefaultTxPoolConfig, params.TestChainConfig, new(event.TypeMux), func() (*state.StateDB, error) { return statedb, nil }, func() *big.Int { return big.NewInt(1000000) })
	defer pool.Stop()
	pool.resetState()

	// Create a test account to add transactions with
	key, _ := crypto.GenerateKey()

	state, _ := pool.currentState()
	state.AddBalance(crypto.PubkeyToAddress(key.PublicKey), big.NewInt(1000000000))

	// Add pending transactions, ensuring the minimum price bump is enforced for replacement (for ultra low prices too)
	price := int64(100)
	threshold := (price * (100 + int64(DefaultTxPoolConfig.PriceBump))) / 100

	if err := pool.AddRemote(pricedTransaction(0, big.NewInt(100000), big.NewInt(1), key)); err != nil {
		t.Fatalf("failed to add original cheap pending transaction: %v", err)
	}
	if err := pool.AddRemote(pricedTransaction(0, big.NewInt(100001), big.NewInt(1), key)); err != ErrReplaceUnderpriced {
		t.Fatalf("original cheap pending transaction replacement error mismatch: have %v, want %v", err, ErrReplaceUnderpriced)
	}
	if err := pool.AddRemote(pricedTransaction(0, big.NewInt(100000), big.NewInt(2), key)); err != nil {
		t.Fatalf("failed to replace original cheap pending transaction: %v", err)
	}

	if err := pool.AddRemote(pricedTransaction(0, big.NewInt(100000), big.NewInt(price), key)); err != nil {
		t.Fatalf("failed to add original proper pending transaction: %v", err)
	}
	if err := pool.AddRemote(pricedTransaction(0, big.NewInt(100000), big.NewInt(threshold), key)); err != ErrReplaceUnderpriced {
		t.Fatalf("original proper pending transaction replacement error mismatch: have %v, want %v", err, ErrReplaceUnderpriced)
	}
	if err := pool.AddRemote(pricedTransaction(0, big.NewInt(100000), big.NewInt(threshold+1), key)); err != nil {
		t.Fatalf("failed to replace original proper pending transaction: %v", err)
	}
	// Add queued transactions, ensuring the minimum price bump is enforced for replacement (for ultra low prices too)
	if err := pool.AddRemote(pricedTransaction(2, big.NewInt(100000), big.NewInt(1), key)); err != nil {
		t.Fatalf("failed to add original queued transaction: %v", err)
	}
	if err := pool.AddRemote(pricedTransaction(2, big.NewInt(100001), big.NewInt(1), key)); err != ErrReplaceUnderpriced {
		t.Fatalf("original queued transaction replacement error mismatch: have %v, want %v", err, ErrReplaceUnderpriced)
	}
	if err := pool.AddRemote(pricedTransaction(2, big.NewInt(100000), big.NewInt(2), key)); err != nil {
		t.Fatalf("failed to replace original queued transaction: %v", err)
	}

	if err := pool.AddRemote(pricedTransaction(2, big.NewInt(100000), big.NewInt(price), key)); err != nil {
		t.Fatalf("failed to add original queued transaction: %v", err)
	}
	if err := pool.AddRemote(pricedTransaction(2, big.NewInt(100001), big.NewInt(threshold), key)); err != ErrReplaceUnderpriced {
		t.Fatalf("original queued transaction replacement error mismatch: have %v, want %v", err, ErrReplaceUnderpriced)
	}
	if err := pool.AddRemote(pricedTransaction(2, big.NewInt(100000), big.NewInt(threshold+1), key)); err != nil {
		t.Fatalf("failed to replace original queued transaction: %v", err)
	}
	if err := validateTxPoolInternals(pool); err != nil {
		t.Fatalf("pool internal state corrupted: %v", err)
	}
}

// Benchmarks the speed of validating the contents of the pending queue of the
// transaction pool.
func BenchmarkPendingDemotion100(b *testing.B)   { benchmarkPendingDemotion(b, 100) }
func BenchmarkPendingDemotion1000(b *testing.B)  { benchmarkPendingDemotion(b, 1000) }
func BenchmarkPendingDemotion10000(b *testing.B) { benchmarkPendingDemotion(b, 10000) }

func benchmarkPendingDemotion(b *testing.B, size int) {
	// Add a batch of transactions to a pool one by one
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))
	state, _ := pool.currentState()
	state.AddBalance(account, big.NewInt(1000000))

	for i := 0; i < size; i++ {
		tx := transaction(uint64(i), big.NewInt(100000), key)
		pool.promoteTx(account, tx.Hash(), tx)
	}
	// Benchmark the speed of pool validation
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.demoteUnexecutables(state)
	}
}

// Benchmarks the speed of scheduling the contents of the future queue of the
// transaction pool.
func BenchmarkFuturePromotion100(b *testing.B)   { benchmarkFuturePromotion(b, 100) }
func BenchmarkFuturePromotion1000(b *testing.B)  { benchmarkFuturePromotion(b, 1000) }
func BenchmarkFuturePromotion10000(b *testing.B) { benchmarkFuturePromotion(b, 10000) }

func benchmarkFuturePromotion(b *testing.B, size int) {
	// Add a batch of transactions to a pool one by one
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))
	state, _ := pool.currentState()
	state.AddBalance(account, big.NewInt(1000000))

	for i := 0; i < size; i++ {
		tx := transaction(uint64(1+i), big.NewInt(100000), key)
		pool.enqueueTx(tx.Hash(), tx)
	}
	// Benchmark the speed of pool validation
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.promoteExecutables(state, nil)
	}
}

// Benchmarks the speed of iterative transaction insertion.
func BenchmarkPoolInsert(b *testing.B) {
	// Generate a batch of transactions to enqueue into the pool
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))
	state, _ := pool.currentState()
	state.AddBalance(account, big.NewInt(1000000))

	txs := make(types.Transactions, b.N)
	for i := 0; i < b.N; i++ {
		txs[i] = transaction(uint64(i), big.NewInt(100000), key)
	}
	// Benchmark importing the transactions into the queue
	b.ResetTimer()
	for _, tx := range txs {
		pool.AddRemote(tx)
	}
}

// Benchmarks the speed of batched transaction insertion.
func BenchmarkPoolBatchInsert100(b *testing.B)   { benchmarkPoolBatchInsert(b, 100) }
func BenchmarkPoolBatchInsert1000(b *testing.B)  { benchmarkPoolBatchInsert(b, 1000) }
func BenchmarkPoolBatchInsert10000(b *testing.B) { benchmarkPoolBatchInsert(b, 10000) }

func benchmarkPoolBatchInsert(b *testing.B, size int) {
	// Generate a batch of transactions to enqueue into the pool
	pool, key := setupTxPool()
	defer pool.Stop()

	account, _ := deriveSender(transaction(0, big.NewInt(0), key))
	state, _ := pool.currentState()
	state.AddBalance(account, big.NewInt(1000000))

	batches := make([]types.Transactions, b.N)
	for i := 0; i < b.N; i++ {
		batches[i] = make(types.Transactions, size)
		for j := 0; j < size; j++ {
			batches[i][j] = transaction(uint64(size*i+j), big.NewInt(100000), key)
		}
	}
	// Benchmark importing the transactions into the queue
	b.ResetTimer()
	for _, batch := range batches {
		pool.AddRemotes(batch)
	}
}
