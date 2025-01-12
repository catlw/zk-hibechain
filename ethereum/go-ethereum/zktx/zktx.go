package zktx

/*
#cgo LDFLAGS: -L/usr/local/lib  -lzk_convert  -lzk_redeem  -lzk_deposit -lzk_withdraw -lff  -lsnark -lstdc++  -lgmp -lgmpxx
#include "convertcgo.hpp"
#include "redeemcgo.hpp"
#include "depositcgo.hpp"
#include "withdrawcgo.hpp"
#include <stdlib.h>
*/
import "C"
import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"sync"
	"unsafe"

	"github.com/ethereum/go-ethereum/crypto/ecies"
	merkle "github.com/ethereum/go-ethereum/merkleTree"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type Sequence struct {
	SN     *common.Hash
	CMT    *common.Hash
	Random *common.Hash
	Value  uint64
	Valid  bool
	Lock   sync.Mutex
	SNbal  *common.Hash
}

type WriteSn struct {
	SNumber      *Sequence
	SNumberAfter *Sequence
}
type SequenceS struct {
	Suquence1 Sequence
	Suquence2 Sequence
	SNS       *Sequence
	PKBX      *big.Int
	PKBY      *big.Int
	Stage     uint8
}

const (
	Origin = iota
	Mint
	Send
	Update
	Deposit
	Redeem
)

const (
	TxConvert = 0
	TxRedeem
	TxDeposit
	TxWithdraw
)

var SNfile *os.File
var FileLine uint8

var Stage uint8
var SequenceNumber = InitializeSN()                //--zy
var SequenceNumberAfter *Sequence = InitializeSN() //--zy
var SNFD *Sequence = nil
var ZKTxAddress = common.HexToAddress("ffffffffffffffffffffffffffffffffffffffff")

var ZKCMTNODES = 1 // max is 32  because of merkle leaves in libnsark is 32

var ErrSequence = errors.New("invalid sequence")
var RandomReceiverPK *ecdsa.PublicKey = nil

func InitializeSN() *Sequence {
	sn := &common.Hash{}
	r := &common.Hash{}
	cmt := GenCMT(0, sn.Bytes(), r.Bytes())
	return &Sequence{
		SN:     sn,
		CMT:    cmt,
		Random: r,
		Value:  0,
	}
}

func NewRandomHash() *common.Hash {
	uuid := make([]byte, 32)
	io.ReadFull(rand.Reader, uuid)
	hash := common.BytesToHash(uuid)
	return &hash
}

func NewRandomAddress() *common.Address {
	uuid := make([]byte, 20)
	io.ReadFull(rand.Reader, uuid)
	addr := common.BytesToAddress(uuid)
	return &addr
}

func NewRandomInt() *big.Int {
	uuid := make([]byte, 32)
	io.ReadFull(rand.Reader, uuid)
	r := big.NewInt(0)
	r.SetBytes(uuid)
	return r
}

var InvalidConvertProof = errors.New("Verifying convert proof failed!!!")

func VerifyConvertProof(cmtold common.Hash, snaold common.Hash, cmtnew common.Hash, value uint64, proof []byte) error {
	cproof := C.CString(string(proof))
	cmtA_old_c := C.CString(common.ToHex(cmtold[:]))
	cmtA_c := C.CString(common.ToHex(cmtnew[:]))
	sn_old_c := C.CString(common.ToHex(snaold.Bytes()[:]))
	value_s_c := C.ulong(value)
	tf := C.verifyConvertproof(cproof, cmtA_old_c, sn_old_c, cmtA_c, value_s_c)
	if tf == false {
		return InvalidConvertProof
	}
	return nil
}

var InvalidRedeemProof = errors.New("Verifying redeem proof failed!!!")

func VerifyRedeemProof(cmtold common.Hash, snaold common.Hash, cmtnew common.Hash, value uint64, proof []byte) error {
	cproof := C.CString(string(proof))
	cmtA_old_c := C.CString(common.ToHex(cmtold[:]))
	cmtA_c := C.CString(common.ToHex(cmtnew[:]))
	sn_old_c := C.CString(common.ToHex(snaold.Bytes()[:]))
	value_s_c := C.ulong(value)

	tf := C.verifyRedeemproof(cproof, cmtA_old_c, sn_old_c, cmtA_c, value_s_c)
	if tf == false {
		return InvalidRedeemProof
	}
	return nil
}

var InvalidDepositProof = errors.New("Verifying deposit proof failed!!!")

func VerifyDepositProof(sna common.Hash, cmts common.Hash, proof []byte, cmtAold common.Hash, cmtAnew common.Hash) error {
	cproof := C.CString(string(proof))
	snAold_c := C.CString(common.ToHex(sna.Bytes()[:]))
	cmtS := C.CString(common.ToHex(cmts[:]))
	cmtAold_c := C.CString(common.ToHex(cmtAold[:]))
	cmtAnew_c := C.CString(common.ToHex(cmtAnew[:]))

	tf := C.verifyDepositproof(cproof, cmtAold_c, snAold_c, cmtS, cmtAnew_c)
	if tf == false {
		return InvalidDepositProof
	}
	return nil
}

var InvalidWithdrawProof = errors.New("Verifying Withdraw proof failed!!!")

func VerifyWithdrawProof(cmtid common.Address, header common.Hash, cmtb common.Hash, snb common.Hash, cmtbnew common.Hash, proof []byte) error {

	pk_c := C.CString(common.ToHex(cmtid[:]))
	cproof := C.CString(string(proof))
	header_c := C.CString(common.ToHex(header[:]))
	cmtB := C.CString(common.ToHex(cmtb[:]))
	cmtBnew := C.CString(common.ToHex(cmtbnew[:]))
	SNB_c := C.CString(common.ToHex(snb.Bytes()[:]))

	fmt.Println("zktx.go VerifyWithdrawProof header ", header_c)
	fmt.Println("zktx.go VerifyWithdrawProof fdid ", pk_c)
	fmt.Println("zktx.go VerifyWithdrawProof cmtB ", cmtB)
	fmt.Println("zktx.go VerifyWithdrawProof SNB ", SNB_c)
	fmt.Println("zktx.go VerifyWithdrawProof cmtBnew ", cmtBnew)

	tf := C.verifyWithdrawproof1(cproof, header_c, pk_c, cmtB, SNB_c, cmtBnew)
	if tf == false {
		return InvalidWithdrawProof
	}
	return nil
}

func VerifyDepositSIG(x *big.Int, y *big.Int, sig []byte) error {
	return nil
}

//GenCMT生成CMT 调用c的sha256函数  （go的sha256函数与c有一些区别）
func GenCMT(value uint64, sn []byte, r []byte) *common.Hash {
	//sn_old_c := C.CString(common.ToHex(SNold[:]))
	value_c := C.ulong(value)
	sn_string := common.ToHex(sn[:])
	sn_c := C.CString(sn_string)
	defer C.free(unsafe.Pointer(sn_c))
	r_string := common.ToHex(r[:])
	r_c := C.CString(r_string)
	defer C.free(unsafe.Pointer(r_c))

	cmtA_c := C.genCMT(value_c, sn_c, r_c)
	cmtA_go := C.GoString(cmtA_c)
	//res := []byte(cmtA_go)
	res, _ := hex.DecodeString(cmtA_go)
	reshash := common.BytesToHash(res)
	return &reshash
}

//GenCMT生成CMT 调用c的sha256函数  （go的sha256函数与c有一些区别）
func GenCMTFD(values uint64, recepientID []byte, sns []byte, rs []byte, sna []byte) *common.Hash {

	values_c := C.ulong(values)

	id_rece := C.CString(common.ToHex(recepientID[:]))
	sns_string := common.ToHex(sns[:])
	sns_c := C.CString(sns_string)
	defer C.free(unsafe.Pointer(sns_c))
	rs_string := common.ToHex(rs[:])
	rs_c := C.CString(rs_string)
	defer C.free(unsafe.Pointer(rs_c))
	sna_string := common.ToHex(sna[:])
	sna_c := C.CString(sna_string)
	defer C.free(unsafe.Pointer(sna_c))
	//uint64_t value_s,char* pk_string,char* sn_s_string,char* r_s_string,char *sn_old_string
	cmtA_c := C.genCMTS(values_c, id_rece, sns_c, rs_c, sna_c) //64长度16进制数
	cmtA_go := C.GoString(cmtA_c)
	//res := []byte(cmtA_go)
	res, _ := hex.DecodeString(cmtA_go)
	reshash := common.BytesToHash(res) //32长度byte数组
	return &reshash
}

//GenCMT生成CMT 调用c的sha256函数  （go的sha256函数与c有一些区别）
func GenCMTS(values uint64, pk *ecdsa.PublicKey, sns []byte, rs []byte, sna []byte) *common.Hash {

	values_c := C.ulong(values)
	PK := crypto.PubkeyToAddress(*pk) //--zy
	pk_c := C.CString(common.ToHex(PK[:]))
	sns_string := common.ToHex(sns[:])
	sns_c := C.CString(sns_string)
	defer C.free(unsafe.Pointer(sns_c))
	rs_string := common.ToHex(rs[:])
	rs_c := C.CString(rs_string)
	defer C.free(unsafe.Pointer(rs_c))
	sna_string := common.ToHex(sna[:])
	sna_c := C.CString(sna_string)
	defer C.free(unsafe.Pointer(sna_c))
	//uint64_t value_s,char* pk_string,char* sn_s_string,char* r_s_string,char *sn_old_string
	cmtA_c := C.genCMTS(values_c, pk_c, sns_c, rs_c, sna_c) //64长度16进制数
	cmtA_go := C.GoString(cmtA_c)
	//res := []byte(cmtA_go)
	res, _ := hex.DecodeString(cmtA_go)
	reshash := common.BytesToHash(res) //32长度byte数组
	return &reshash
}

// //GenRT 返回merkel树的hash  --zy
// func GenRT(CMTSForMerkle []*common.Hash) common.Hash {
// 	var cmtArray string
// 	for i := 0; i < len(CMTSForMerkle); i++ {
// 		s := string(common.ToHex(CMTSForMerkle[i][:]))
// 		cmtArray += s
// 	}
// 	cmtsM := C.CString(cmtArray)
// 	rtC := C.genRoot(cmtsM, C.int(len(CMTSForMerkle))) //--zy
// 	rtGo := C.GoString(rtC)

// 	res, _ := hex.DecodeString(rtGo)   //返回32长度 []byte  一个byte代表两位16进制数
// 	reshash := common.BytesToHash(res) //32长度byte数组
// 	return reshash
// }

func ComputeR(sk *big.Int) *ecdsa.PublicKey {
	return &ecdsa.PublicKey{} //tbd
}

func Encrypt(pub *ecdsa.PublicKey, m []byte) ([]byte, error) {
	P := ecies.ImportECDSAPublic(pub)
	ke := P.X.Bytes()
	ke = ke[:16]
	ct, err := ecies.SymEncrypt(rand.Reader, P.Params, ke, m)

	return ct, err
}

func Decrypt(pub *ecdsa.PublicKey, ct []byte) ([]byte, error) {
	P := ecies.ImportECDSAPublic(pub)
	ke := P.X.Bytes()
	ke = ke[:16]
	m, err := ecies.SymDecrypt(P.Params, ke, ct)
	return m, err
}

type AUX struct {
	Value uint64
	SNs   *common.Hash
	Rs    *common.Hash
	SNa   *common.Hash
}

func ComputeAUX(randomReceiverPK *ecdsa.PublicKey, value uint64, SNs *common.Hash, Rs *common.Hash, SNa *common.Hash) []byte {
	aux := AUX{
		Value: value,
		SNs:   SNs,
		Rs:    Rs,
		SNa:   SNa,
	}
	bytes, _ := rlp.EncodeToBytes(aux)
	encbytes, _ := Encrypt(randomReceiverPK, bytes)
	return encbytes
}

func DecAUX(key *ecdsa.PublicKey, data []byte) (uint64, *common.Hash, *common.Hash, *common.Hash) {
	decdata, _ := Decrypt(key, data)
	aux := AUX{}
	r := bytes.NewReader(decdata)

	s := rlp.NewStream(r, 128)
	if err := s.Decode(&aux); err != nil {
		fmt.Println("Decode aux error: ", err)
		return 0, nil, nil, nil
	}
	return aux.Value, aux.SNs, aux.Rs, aux.SNa
}

func GenerateKeyForRandomB(R *ecdsa.PublicKey, kB *ecdsa.PrivateKey) *ecdsa.PrivateKey {
	//skB*R
	c := kB.PublicKey.Curve
	tx, ty := c.ScalarMult(R.X, R.Y, kB.D.Bytes())
	tmp := tx.Bytes()
	tmp = append(tmp, ty.Bytes()...)

	//生成hash值H(skB*R)
	h := sha256.New()
	h.Write([]byte(tmp))
	bs := h.Sum(nil)
	bs[0] = bs[0] % 128
	i := new(big.Int)
	i = i.SetBytes(bs)

	//生成公钥
	sx, sy := c.ScalarBaseMult(bs)
	sskB := new(ecdsa.PrivateKey)
	sskB.PublicKey.X, sskB.PublicKey.Y = c.Add(sx, sy, kB.PublicKey.X, kB.PublicKey.Y)
	sskB.Curve = c
	//生成私钥
	sskB.D = i.Add(i, kB.D)
	return sskB
}

func GenConvertProof(CMTold *common.Hash, SNold *common.Hash, RAold *common.Hash, ValueOld uint64, CMTnew *common.Hash, SNAnew *common.Hash, RAnew *common.Hash, ValueNew uint64) []byte {
	value_c := C.ulong(ValueNew)     //转换后零知识余额对应的明文余额
	value_old_c := C.ulong(ValueOld) //转换前零知识余额对应的明文余额

	sn_old_c := C.CString(common.ToHex(SNold[:]))
	r_old_c := C.CString(common.ToHex(RAold[:]))
	sn_c := C.CString(common.ToHex(SNAnew[:]))
	r_c := C.CString(common.ToHex(RAnew[:]))

	cmtA_old_c := C.CString(common.ToHex(CMTold[:])) //对于CMT  需要将每一个byte拆为两个16进制字符
	cmtA_c := C.CString(common.ToHex(CMTnew[:]))

	value_s_c := C.ulong(ValueNew - ValueOld) //需要被转换的明文余额

	cproof := C.genConvertproof(value_c, value_old_c, sn_old_c, r_old_c, sn_c, r_c, cmtA_old_c, cmtA_c, value_s_c)

	var goproof string
	goproof = C.GoString(cproof)
	return []byte(goproof)
}

func GenRedeemProof(CMTold *common.Hash, SNold *common.Hash, RAold *common.Hash, ValueOld uint64, CMTnew *common.Hash, SNAnew *common.Hash, RAnew *common.Hash, ValueNew uint64) []byte {
	value_c := C.ulong(ValueNew)     //转换后零知识余额对应的明文余额
	value_old_c := C.ulong(ValueOld) //转换前零知识余额对应的明文余额

	sn_old_c := C.CString(common.ToHex(SNold.Bytes()[:]))
	r_old_c := C.CString(common.ToHex(RAold.Bytes()[:]))
	sn_c := C.CString(common.ToHex(SNAnew.Bytes()[:]))
	r_c := C.CString(common.ToHex(RAnew.Bytes()[:]))

	cmtA_old_c := C.CString(common.ToHex(CMTold[:])) //对于CMT  需要将每一个byte拆为两个16进制字符
	cmtA_c := C.CString(common.ToHex(CMTnew[:]))

	value_s_c := C.ulong(ValueOld - ValueNew) //需要被转换的明文余额

	cproof := C.genRedeemproof(value_c, value_old_c, sn_old_c, r_old_c, sn_c, r_c, cmtA_old_c, cmtA_c, value_s_c)

	var goproof string
	goproof = C.GoString(cproof)
	return []byte(goproof)
}

func GenDepositProof(CMTA *common.Hash, ValueA uint64, RA *common.Hash, ValueS uint64, IDrece common.Address, SNS *common.Hash, RS *common.Hash, SNA *common.Hash, CMTS *common.Hash, ValueAnew uint64, SNAnew *common.Hash, RAnew *common.Hash, CMTAnew *common.Hash) []byte {
	cmtA_c := C.CString(common.ToHex(CMTA[:]))
	valueA_c := C.ulong(ValueA)
	rA_c := C.CString(common.ToHex(RA.Bytes()[:]))
	valueS := C.ulong(ValueS)

	pk_c := C.CString(common.ToHex(IDrece[:]))
	snS := C.CString(common.ToHex(SNS.Bytes()[:]))
	rS := C.CString(common.ToHex(RS.Bytes()[:]))
	snA := C.CString(common.ToHex(SNA.Bytes()[:]))
	cmtS := C.CString(common.ToHex(CMTS[:]))
	//ValueAnew uint64 , SNAnew *common.Hash, RAnew *common.Hash,CMTAnew *common.Hash
	valueANew_c := C.ulong(ValueAnew)
	snAnew_c := C.CString(common.ToHex(SNAnew.Bytes()[:]))
	rAnew_c := C.CString(common.ToHex(RAnew.Bytes()[:]))
	cmtAnew_c := C.CString(common.ToHex(CMTAnew[:]))

	cproof := C.genDepositproof(valueA_c, snS, rS, snA, rA_c, cmtS, cmtA_c, valueS, pk_c, valueANew_c, snAnew_c, rAnew_c, cmtAnew_c)
	var goproof string
	goproof = C.GoString(cproof)
	return []byte(goproof)
}

type BlockHead struct {
	TxRoot      common.Hash
	StateRoot   common.Hash
	FdRoot      common.Hash
	Header      common.Hash
	BlockNumber uint64
}

type TxField struct {
	ZKSN   common.Hash
	ZKbal  common.Hash
	Header common.Hash
}

type TxInBlock struct {
	Txs     []common.Hash
	Txindex uint32
}
type FDPath struct {
	Funds         []common.Hash
	FundIndex     uint32
	FundsRoot     common.Hash
	BlockHeads    []BlockHead
	TxFields      []TxField
	TxInBlocks    []TxInBlock
	RootBlockHash common.Hash
	Depth         uint32
}

func GenWithdrawProof1(CMTS common.Hash, ValueS uint64, SNS common.Hash, RS common.Hash, SNA common.Hash, ValueB uint64, CMTB common.Hash, RB common.Hash, SNB common.Hash, CMTBnew common.Hash, SNBnew common.Hash, RBnew common.Hash, toaddress []byte, path FDPath) []byte {
	block1 := path.BlockHeads[0]
	txrootstring := C.CString(common.ToHex(block1.TxRoot[:]))
	staterootstring := C.CString(common.ToHex(block1.StateRoot[:]))
	RTcmt := path.FundsRoot
	cmtS_c := C.CString(common.ToHex(CMTS[:]))
	valueS_c := C.ulong(ValueS)
	pk_c := C.CString(common.ToHex(toaddress[:]))
	SNS_c := C.CString(common.ToHex(SNS.Bytes()[:])) //--zy
	RS_c := C.CString(common.ToHex(RS.Bytes()[:]))   //--zy
	SNA_c := C.CString(common.ToHex(SNA.Bytes()[:]))
	valueB_c := C.ulong(ValueB)
	RB_c := C.CString(common.ToHex(RB.Bytes()[:])) //rA_c := C.CString(string(RA.Bytes()[:]))
	SNB_c := C.CString(common.ToHex(SNB.Bytes()[:]))
	SNBnew_c := C.CString(common.ToHex(SNBnew.Bytes()[:]))
	RBnew_c := C.CString(common.ToHex(RBnew.Bytes()[:]))
	cmtB_c := C.CString(common.ToHex(CMTB[:]))
	fdrootstring := C.CString(common.ToHex(RTcmt.Bytes())) //--zy   rt

	cmtBnew_c := C.CString(common.ToHex(CMTBnew[:]))
	valueBNew_c := C.ulong(ValueB + ValueS)

	headerstring := C.CString(common.ToHex(block1.Header[:]))
	CMTSForMerkle := path.Funds

	var cmtArray string
	fmt.Println("zktx.go  CMTSForMerkle")
	for i := 0; i < len(CMTSForMerkle); i++ {
		s := string(common.ToHex(CMTSForMerkle[i][:]))
		cmtArray += s
		fmt.Println(s)
	}
	for i := len(CMTSForMerkle); i < merkle.ZkfundsMerkleNODES; i++ {
		cmtArray += "0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
		fmt.Println("0xffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
	}

	cmtsM := C.CString(cmtArray)
	nC := C.int(len(CMTSForMerkle))

	fmt.Println("zktx.go  fdroot", fdrootstring)
	fmt.Println("zktx.go  txroot", txrootstring)
	fmt.Println("zktx.go  stateroot", staterootstring)
	fmt.Println("zktx.go  header", headerstring)

	cproof := C.genWithdrawproof1(txrootstring, staterootstring, valueBNew_c, valueB_c, SNB_c, RB_c, SNBnew_c, RBnew_c, SNS_c, RS_c, cmtB_c, cmtBnew_c, valueS_c, pk_c, SNA_c, cmtS_c, cmtsM, nC, headerstring)
	var goproof string
	goproof = C.GoString(cproof)
	return []byte(goproof)
}

func GenWithdrawProof(CMTS common.Hash, ValueS uint64, SNS common.Hash, RS common.Hash, SNA common.Hash, ValueB uint64, CMTB *common.Hash, RB *common.Hash, SNB *common.Hash, CMTBnew *common.Hash, SNBnew *common.Hash, RBnew *common.Hash, toaddress []byte, path FDPath) []byte {
	var proof []byte
	switch path.Depth {
	case 1:
		proof = GenWithdrawProof1(CMTS, ValueS, SNS, RS, SNA, ValueB, *CMTB, *RB, *SNB, *CMTBnew, *SNBnew, *RBnew, toaddress, path)
	}
	return proof
}

func GenR() *ecdsa.PrivateKey {
	Ka, err := crypto.GenerateKey()
	if err != nil {
		return nil
	}
	return Ka
}

func NewRandomPubKey(sA *big.Int, pkB ecdsa.PublicKey) *ecdsa.PublicKey {
	//sA*pkB
	c := pkB.Curve
	tx, ty := c.ScalarMult(pkB.X, pkB.Y, sA.Bytes())
	tmp := tx.Bytes()
	tmp = append(tmp, ty.Bytes()...)

	//生成hash值H(sA*pkB)
	h := sha256.New()
	h.Write([]byte(tmp))
	bs := h.Sum(nil)
	bs[0] = bs[0] % 128

	//生成用于加密的公钥H(sA*pkB)P+pkB
	sx, sy := c.ScalarBaseMult(bs)
	spkB := new(ecdsa.PublicKey)
	spkB.X, spkB.Y = c.Add(sx, sy, pkB.X, pkB.Y)
	spkB.Curve = c
	return spkB
}
