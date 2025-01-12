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
//=>add pbft --Agzs
// package web3ext contains geth specific web3.js extensions.
package web3ext

var Modules = map[string]string{
	"admin":      Admin_JS,
	"chequebook": Chequebook_JS,
	"clique":     Clique_JS,
	"pbft":       PBFT_JS,
	"debug":      Debug_JS,
	"eth":        Eth_JS,
	"miner":      Miner_JS,
	"net":        Net_JS,
	"personal":   Personal_JS,
	"rpc":        RPC_JS,
	"shh":        Shh_JS,
	"swarmfs":    SWARMFS_JS,
	"txpool":     TxPool_JS,
}

const Chequebook_JS = `
web3._extend({
  property: 'chequebook',
  methods:
  [
    new web3._extend.Method({
      name: 'deposit',
      call: 'chequebook_deposit',
      params: 1,
      inputFormatter: [null]
    }),
    new web3._extend.Property({
			name: 'balance',
			getter: 'chequebook_balance',
				outputFormatter: web3._extend.utils.toDecimal
		}),
    new web3._extend.Method({
      name: 'cash',
      call: 'chequebook_cash',
      params: 1,
      inputFormatter: [null]
    }),
    new web3._extend.Method({
      name: 'issue',
      call: 'chequebook_issue',
      params: 2,
      inputFormatter: [null, null]
    }),
  ]
});
`

const Clique_JS = `
web3._extend({
  property: 'clique',
  methods:
  [
		new web3._extend.Method({
			name: 'getSnapshot',
			call: 'clique_getSnapshot',
			params: 1,
      inputFormatter: [null]
		}),
		new web3._extend.Method({
			name: 'getSnapshotAtHash',
			call: 'clique_getSnapshotAtHash',
			params: 1
		}),
    new web3._extend.Method({
      name: 'getSigners',
      call: 'clique_getSigners',
      params: 1,
      inputFormatter: [null]
    }),
		new web3._extend.Method({
			name: 'getSignersAtHash',
			call: 'clique_getSignersAtHash',
			params: 1
		}),
		new web3._extend.Method({
			name: 'propose',
			call: 'clique_propose',
			params: 2
		}),
		new web3._extend.Method({
			name: 'discard',
			call: 'clique_discard',
			params: 1
		})
  ],
	properties:
	[
		new web3._extend.Property({
			name: 'proposals',
			getter: 'clique_proposals'
		}),
	]
});
`

const PBFT_JS = `
web3._extend({
  property: 'pbft',
  methods:
  [
		new web3._extend.Method({
			name: 'getSnapshot',
			call: 'pbft_getSnapshot',
			params: 1,
      inputFormatter: [null]
		}),
		new web3._extend.Method({
			name: 'getSnapshotAtHash',
			call: 'pbft_getSnapshotAtHash',
			params: 1
		}),
    new web3._extend.Method({
      name: 'getSigners',
      call: 'pbft_getSigners',
      params: 1,
      inputFormatter: [null]
    }),
		new web3._extend.Method({
			name: 'getSignersAtHash',
			call: 'pbft_getSignersAtHash',
			params: 1
		}),
		new web3._extend.Method({
			name: 'propose',
			call: 'pbft_propose',
			params: 2
		}),
		new web3._extend.Method({
			name: 'discard',
			call: 'pbft_discard',
			params: 1
		})
  ],
	properties:
	[
		new web3._extend.Property({
			name: 'proposals',
			getter: 'pbft_proposals'
		}),
	]
});
`

const Admin_JS = `
web3._extend({
	property: 'admin',
	methods:
	[
		new web3._extend.Method({
			name: 'setID',
			call: 'admin_setID',
			params: 1
		}),
		new web3._extend.Method({
			name: 'keyStatus',
			call: 'admin_keyStatus'
		}),
		new web3._extend.Method({
			name: 'iD',
			call: 'admin_iD'
		}),
		new web3._extend.Method({
			name: 'setLevel',
			call: 'admin_setLevel',
			params: 2
		}),
		new web3._extend.Method({
			name: 'setNumber',
			call: 'admin_setNumber',
			params: 2
		}),
		new web3._extend.Method({
			name: 'testhibe',
			call: 'admin_testhibe',
			params: 1
		}),
		new web3._extend.Method({
			name: 'addPeerFromFile',
			call: 'admin_addPeerFromFile',
			params: 1
		}),
		new web3._extend.Method({
			name: 'addPeer',
			call: 'admin_addPeer',
			params: 2
		}),
		new web3._extend.Method({
			name: 'removePeer',
			call: 'admin_removePeer',
			params: 2
		}),
		new web3._extend.Method({
			name: 'exportChain',
			call: 'admin_exportChain',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Method({
			name: 'importChain',
			call: 'admin_importChain',
			params: 1
		}),
		new web3._extend.Method({
			name: 'sleepBlocks',
			call: 'admin_sleepBlocks',
			params: 2
		}),
		new web3._extend.Method({
			name: 'startRPC',
			call: 'admin_startRPC',
			params: 4,
			inputFormatter: [null, null, null, null]
		}),
		new web3._extend.Method({
			name: 'stopRPC',
			call: 'admin_stopRPC'
		}),
		new web3._extend.Method({
			name: 'startWS',
			call: 'admin_startWS',
			params: 4,
			inputFormatter: [null, null, null, null]
		}),
		new web3._extend.Method({
			name: 'stopWS',
			call: 'admin_stopWS'
		})
	],
	properties:
	[
		new web3._extend.Property({
			name: 'nodeInfo',
			getter: 'admin_nodeInfo'
		}),
		new web3._extend.Property({
			name: 'peers',
			getter: 'admin_peers'
		}),
		new web3._extend.Property({
			name: 'datadir',
			getter: 'admin_datadir'
		})
	]
});
`

const Debug_JS = `
web3._extend({
	property: 'debug',
	methods:
	[
		new web3._extend.Method({
			name: 'printBlock',
			call: 'debug_printBlock',
			params: 1
		}),
		new web3._extend.Method({
			name: 'getBlockRlp',
			call: 'debug_getBlockRlp',
			params: 1
		}),
		new web3._extend.Method({
			name: 'setHead',
			call: 'debug_setHead',
			params: 1
		}),
		new web3._extend.Method({
			name: 'traceBlock',
			call: 'debug_traceBlock',
			params: 1
		}),
		new web3._extend.Method({
			name: 'traceBlockFromFile',
			call: 'debug_traceBlockFromFile',
			params: 1
		}),
		new web3._extend.Method({
			name: 'traceBlockByNumber',
			call: 'debug_traceBlockByNumber',
			params: 1
		}),
		new web3._extend.Method({
			name: 'traceBlockByHash',
			call: 'debug_traceBlockByHash',
			params: 1
		}),
		new web3._extend.Method({
			name: 'seedHash',
			call: 'debug_seedHash',
			params: 1
		}),
		new web3._extend.Method({
			name: 'dumpBlock',
			call: 'debug_dumpBlock',
			params: 1
		}),
		new web3._extend.Method({
			name: 'chaindbProperty',
			call: 'debug_chaindbProperty',
			params: 1,
			outputFormatter: console.log
		}),
		new web3._extend.Method({
			name: 'chaindbCompact',
			call: 'debug_chaindbCompact',
		}),
		new web3._extend.Method({
			name: 'metrics',
			call: 'debug_metrics',
			params: 1
		}),
		new web3._extend.Method({
			name: 'verbosity',
			call: 'debug_verbosity',
			params: 1
		}),
		new web3._extend.Method({
			name: 'vmodule',
			call: 'debug_vmodule',
			params: 1
		}),
		new web3._extend.Method({
			name: 'backtraceAt',
			call: 'debug_backtraceAt',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'stacks',
			call: 'debug_stacks',
			params: 0,
			outputFormatter: console.log
		}),
		new web3._extend.Method({
			name: 'memStats',
			call: 'debug_memStats',
			params: 0,
		}),
		new web3._extend.Method({
			name: 'gcStats',
			call: 'debug_gcStats',
			params: 0,
		}),
		new web3._extend.Method({
			name: 'cpuProfile',
			call: 'debug_cpuProfile',
			params: 2
		}),
		new web3._extend.Method({
			name: 'startCPUProfile',
			call: 'debug_startCPUProfile',
			params: 1
		}),
		new web3._extend.Method({
			name: 'stopCPUProfile',
			call: 'debug_stopCPUProfile',
			params: 0
		}),
		new web3._extend.Method({
			name: 'goTrace',
			call: 'debug_goTrace',
			params: 2
		}),
		new web3._extend.Method({
			name: 'startGoTrace',
			call: 'debug_startGoTrace',
			params: 1
		}),
		new web3._extend.Method({
			name: 'stopGoTrace',
			call: 'debug_stopGoTrace',
			params: 0
		}),
		new web3._extend.Method({
			name: 'blockProfile',
			call: 'debug_blockProfile',
			params: 2
		}),
		new web3._extend.Method({
			name: 'setBlockProfileRate',
			call: 'debug_setBlockProfileRate',
			params: 1
		}),
		new web3._extend.Method({
			name: 'writeBlockProfile',
			call: 'debug_writeBlockProfile',
			params: 1
		}),
		new web3._extend.Method({
			name: 'writeMemProfile',
			call: 'debug_writeMemProfile',
			params: 1
		}),
		new web3._extend.Method({
			name: 'traceTransaction',
			call: 'debug_traceTransaction',
			params: 2,
			inputFormatter: [null, null]
		}),
		new web3._extend.Method({
			name: 'preimage',
			call: 'debug_preimage',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Method({
			name: 'getBadBlocks',
			call: 'debug_getBadBlocks',
			params: 0,
		}),
		new web3._extend.Method({
			name: 'storageRangeAt',
			call: 'debug_storageRangeAt',
			params: 5,
		}),
	],
	properties: []
});
`

////add `sendBatchTransactions`、`stopSend`. --xiaobei  2.4
const Eth_JS = `
web3._extend({
	property: 'eth',
	methods:
	[
		new web3._extend.Method({
			name: 'getBalances',
			call: 'eth_getBalances',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'sendConvertTransaction',
			call: 'eth_sendConvertTransaction',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'sendRedeemTransaction',
			call: 'eth_sendRedeemTransaction',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'sendDepositTransaction',
			call: 'eth_sendDepositTransaction',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'sendWithdrawTransaction',
			call: 'eth_sendWithdrawTransaction',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'extractPath',
			call: 'eth_extractPath',
			params: 1,
		}),

		new web3._extend.Method({
			name: 'stopSend',
			call: 'eth_stopSend',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Method({
			name: 'sendBatchTransactions',
			call: 'eth_sendBatchTransactions',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter]
		}),
		new web3._extend.Method({
			name: 'sendTransaction2',
			call: 'eth_sendTransaction2',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'sendTransaction3',
			call: 'eth_sendTransaction3',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'keyCount',
			call: 'eth_keyCount',
		}),
		new web3._extend.Method({
			name: 'removeTx',
			call: 'eth_removeTx',
		}),
		new web3._extend.Method({
			name: 'testSendTransaction2',
			call: 'eth_testSendTransaction2',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'requestCrossChainTransation',
			call: 'eth_requestCrossChainTransation',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'getTxProofByHash',
			call: 'eth_getTxProofByHash',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'getTxProofByProof',
			call: 'eth_getTxProofByProof',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'getFund',
			call: 'eth_getFund',
			params: 1,
		}),
		new web3._extend.Method({
			name: 'sign',
			call: 'eth_sign',
			params: 2,
			inputFormatter: [web3._extend.formatters.inputAddressFormatter, null]
		}),
		new web3._extend.Method({
			name: 'resend',
			call: 'eth_resend',
			params: 3,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter, web3._extend.utils.fromDecimal, web3._extend.utils.fromDecimal]
		}),
		new web3._extend.Method({
			name: 'signTransaction',
			call: 'eth_signTransaction',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter]
		}),
		new web3._extend.Method({
			name: 'submitTransaction',
			call: 'eth_submitTransaction',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputTransactionFormatter]
		}),
		new web3._extend.Method({
			name: 'getRawTransaction',
			call: 'eth_getRawTransactionByHash',
			params: 1
		}),
		new web3._extend.Method({
			name: 'getRawTransactionFromBlock',
			call: function(args) {
				return (web3._extend.utils.isString(args[0]) && args[0].indexOf('0x') === 0) ? 'eth_getRawTransactionByBlockHashAndIndex' : 'eth_getRawTransactionByBlockNumberAndIndex';
			},
			params: 2,
			inputFormatter: [web3._extend.formatters.inputBlockNumberFormatter, web3._extend.utils.toHex]
		})
	],
	properties:
	[
		new web3._extend.Property({
			name: 'pendingTransactions',
			getter: 'eth_pendingTransactions',
			outputFormatter: function(txs) {
				var formatted = [];
				for (var i = 0; i < txs.length; i++) {
					formatted.push(web3._extend.formatters.outputTransactionFormatter(txs[i]));
					formatted[i].blockHash = null;
				}
				return formatted;
			}
		})
	]
});
`

const Miner_JS = `
web3._extend({
	property: 'miner',
	methods:
	[
		new web3._extend.Method({
			name: 'start',
			call: 'miner_start',
			params: 1,
			inputFormatter: [null]
		}),
		new web3._extend.Method({
			name: 'stop',
			call: 'miner_stop'
		}),
		new web3._extend.Method({
			name: 'setEtherbase',
			call: 'miner_setEtherbase',
			params: 1,
			inputFormatter: [web3._extend.formatters.inputAddressFormatter]
		}),
		new web3._extend.Method({
			name: 'setExtra',
			call: 'miner_setExtra',
			params: 1
		}),
		new web3._extend.Method({
			name: 'setGasPrice',
			call: 'miner_setGasPrice',
			params: 1,
			inputFormatter: [web3._extend.utils.fromDecimal]
		}),
		new web3._extend.Method({
			name: 'getHashrate',
			call: 'miner_getHashrate'
		})
	],
	properties: []
});
`

const Net_JS = `
web3._extend({
	property: 'net',
	methods: [],
	properties:
	[
		new web3._extend.Property({
			name: 'version',
			getter: 'net_version'
		})
	]
});
`

const Personal_JS = `
web3._extend({
	property: 'personal',
	methods:
	[
		new web3._extend.Method({
			name: 'importRawKey',
			call: 'personal_importRawKey',
			params: 2
		}),
		new web3._extend.Method({
			name: 'sign',
			call: 'personal_sign',
			params: 3,
			inputFormatter: [null, web3._extend.formatters.inputAddressFormatter, null]
		}),
		new web3._extend.Method({
			name: 'ecRecover',
			call: 'personal_ecRecover',
			params: 2
		}),
		new web3._extend.Method({
			name: 'deriveAccount',
			call: 'personal_deriveAccount',
			params: 3
		})
	],
	properties:
	[
		new web3._extend.Property({
			name: 'listWallets',
			getter: 'personal_listWallets'
		})
	]
})
`

const RPC_JS = `
web3._extend({
	property: 'rpc',
	methods: [],
	properties:
	[
		new web3._extend.Property({
			name: 'modules',
			getter: 'rpc_modules'
		})
	]
});
`

const Shh_JS = `
web3._extend({
	property: 'shh',
	methods: [
		new web3._extend.Method({
			name: 'setMaxMessageLength',
			call: 'shh_setMaxMessageLength',
			params: 1
		}),
		new web3._extend.Method({
			name: 'setMinimumPoW',
			call: 'shh_setMinimumPoW',
			params: 1
		}),
		new web3._extend.Method({
			name: 'markTrustedPeer',
			call: 'shh_markTrustedPeer',
			params: 1
		}),
		new web3._extend.Method({
			name: 'hasKeyPair',
			call: 'shh_hasKeyPair',
			params: 1
		}),
		new web3._extend.Method({
			name: 'deleteKeyPair',
			call: 'shh_deleteKeyPair',
			params: 1
		}),
		new web3._extend.Method({
			name: 'newKeyPair',
			call: 'shh_newKeyPair'
		}),
		new web3._extend.Method({
			name: 'getPublicKey',
			call: 'shh_getPublicKey',
			params: 1
		}),
		new web3._extend.Method({
			name: 'getPrivateKey',
			call: 'shh_getPrivateKey',
			params: 1
		}),
		new web3._extend.Method({
			name: 'newSymKey',
			call: 'shh_newSymKey',
		}),
		new web3._extend.Method({
			name: 'addSymKey',
			call: 'shh_addSymKey',
			params: 1
		}),
		new web3._extend.Method({
			name: 'generateSymKeyFromPassword',
			call: 'shh_generateSymKeyFromPassword',
			params: 1
		}),
		new web3._extend.Method({
			name: 'hasSymKey',
			call: 'shh_hasSymKey',
			params: 1
		}),
		new web3._extend.Method({
			name: 'getSymKey',
			call: 'shh_getSymKey',
			params: 1
		}),
		new web3._extend.Method({
			name: 'deleteSymKey',
			call: 'shh_deleteSymKey',
			params: 1
		}),
		new web3._extend.Method({
			name: 'subscribe',
			call: 'shh_subscribe',
			params: 2
		}),
		new web3._extend.Method({
			name: 'unsubscribe',
			call: 'shh_unsubscribe',
			params: 1
		}),
		new web3._extend.Method({
			name: 'post',
			call: 'shh_post',
			params: 1
		}),
		new web3._extend.Method({
			name: 'publicKey',
			call: 'shh_getPublicKey',
			params: 1
		}),
		new web3._extend.Method({
			name: 'getFilterMessages',
			call: 'shh_getFilterMessages',
			params: 1
		}),
		new web3._extend.Method({
			name: 'deleteMessageFilter',
			call: 'shh_deleteMessageFilter',
			params: 1
		}),
		new web3._extend.Method({
			name: 'newMessageFilter',
			call: 'shh_newMessageFilter',
			params: 1
		})
	],
	properties:
	[
		new web3._extend.Property({
			name: 'version',
			getter: 'shh_version',
			outputFormatter: web3._extend.utils.toDecimal
		}),
		new web3._extend.Property({
			name: 'info',
			getter: 'shh_info'
		}),
	]
});
`

const SWARMFS_JS = `
web3._extend({
	property: 'swarmfs',
	methods:
	[
		new web3._extend.Method({
			name: 'mount',
			call: 'swarmfs_mount',
			params: 2
		}),
		new web3._extend.Method({
			name: 'unmount',
			call: 'swarmfs_unmount',
			params: 1
		}),
		new web3._extend.Method({
			name: 'listmounts',
			call: 'swarmfs_listmounts',
			params: 0
		})
	]
});
`

const TxPool_JS = `
web3._extend({
	property: 'txpool',
	methods: [],
	properties:
	[
		new web3._extend.Property({
			name: 'content',
			getter: 'txpool_content'
		}),
		new web3._extend.Property({
			name: 'inspect',
			getter: 'txpool_inspect'
		}),
		new web3._extend.Property({
			name: 'status',
			getter: 'txpool_status',
			outputFormatter: function(status) {
				status.pending = web3._extend.utils.toDecimal(status.pending);
				status.queued = web3._extend.utils.toDecimal(status.queued);
				return status;
			}
		})
	]
});
`
