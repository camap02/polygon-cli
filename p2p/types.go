package p2p

import (
	"container/list"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/eth/protocols/snap"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/rlpx"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/rs/zerolog"
)

type Message interface {
	Code() int
	ReqID() uint64
}

type Error struct {
	err error
}

func (e *Error) Unwrap() error  { return e.err }
func (e *Error) Error() string  { return e.err.Error() }
func (e *Error) String() string { return e.Error() }

func (e *Error) Code() int     { return -1 }
func (e *Error) ReqID() uint64 { return 0 }

func errorf(format string, args ...interface{}) *Error {
	return &Error{fmt.Errorf(format, args...)}
}

// Hello is the RLP structure of the protocol handshake.
type Hello struct {
	Version    uint64
	Name       string
	Caps       []p2p.Cap
	ListenPort uint64
	ID         []byte // secp256k1 public key

	// Ignore additional fields (for forward compatibility).
	Rest []rlp.RawValue `rlp:"tail"`
}

func (msg Hello) Code() int     { return 0x00 }
func (msg Hello) ReqID() uint64 { return 0 }

// Disconnect is the RLP structure for a disconnect message.
type Disconnect struct {
	Reason p2p.DiscReason
}

func (msg Disconnect) Code() int     { return 0x01 }
func (msg Disconnect) ReqID() uint64 { return 0 }

// Disconnects is the RLP structure for disconnect messages.
type Disconnects []p2p.DiscReason

func (msg Disconnects) Code() int     { return 0x01 }
func (msg Disconnects) ReqID() uint64 { return 0 }

type Ping struct{}

func (msg Ping) Code() int     { return 0x02 }
func (msg Ping) ReqID() uint64 { return 0 }

type Pong struct{}

func (msg Pong) Code() int     { return 0x03 }
func (msg Pong) ReqID() uint64 { return 0 }

// Status is the network packet for the status message for eth/64 and later.
type Status eth.StatusPacket

func (msg Status) Code() int     { return 16 }
func (msg Status) ReqID() uint64 { return 0 }

// NewBlockHashes is the network packet for the block announcements.
type NewBlockHashes eth.NewBlockHashesPacket

func (msg NewBlockHashes) Code() int     { return 17 }
func (msg NewBlockHashes) ReqID() uint64 { return 0 }

type Transactions eth.TransactionsPacket

func (msg Transactions) Code() int     { return 18 }
func (msg Transactions) ReqID() uint64 { return 18 }

// GetBlockHeaders represents a block header query.
type GetBlockHeaders eth.GetBlockHeadersPacket66

func (msg GetBlockHeaders) Code() int     { return 19 }
func (msg GetBlockHeaders) ReqID() uint64 { return msg.RequestId }

type BlockHeaders eth.BlockHeadersPacket66

func (msg BlockHeaders) Code() int     { return 20 }
func (msg BlockHeaders) ReqID() uint64 { return msg.RequestId }

// GetBlockBodies represents a GetBlockBodies request
type GetBlockBodies eth.GetBlockBodiesPacket66

func (msg GetBlockBodies) Code() int     { return 21 }
func (msg GetBlockBodies) ReqID() uint64 { return msg.RequestId }

// BlockBodies is the network packet for block content distribution.
type BlockBodies eth.BlockBodiesPacket66

func (msg BlockBodies) Code() int     { return 22 }
func (msg BlockBodies) ReqID() uint64 { return msg.RequestId }

// NewBlock is the network packet for the block propagation message.
type NewBlock eth.NewBlockPacket

func (msg NewBlock) Code() int     { return 23 }
func (msg NewBlock) ReqID() uint64 { return 0 }

// NewPooledTransactionHashes66 is the network packet for the tx hash propagation message.
type NewPooledTransactionHashes66 []common.Hash

func (msg NewPooledTransactionHashes66) Code() int     { return 24 }
func (msg NewPooledTransactionHashes66) ReqID() uint64 { return 0 }

// NewPooledTransactionHashes is the network packet for the tx hash propagation message.
type NewPooledTransactionHashes struct {
	Types  []byte
	Sizes  []uint32
	Hashes []common.Hash
}

func (msg NewPooledTransactionHashes) Code() int     { return 24 }
func (msg NewPooledTransactionHashes) ReqID() uint64 { return 0 }

type GetPooledTransactions eth.GetPooledTransactionsPacket66

func (msg GetPooledTransactions) Code() int     { return 25 }
func (msg GetPooledTransactions) ReqID() uint64 { return msg.RequestId }

type PooledTransactions eth.PooledTransactionsPacket66

func (msg PooledTransactions) Code() int     { return 26 }
func (msg PooledTransactions) ReqID() uint64 { return msg.RequestId }

// Conn represents an individual connection with a peer
type Conn struct {
	*rlpx.Conn
	SensorID string

	ourKey *ecdsa.PrivateKey
	caps   []p2p.Cap
	node   *enode.Node
	logger zerolog.Logger

	// requests is used to store the request ID and the block hash. This is used
	// when fetching block bodies because the eth protocol block bodies do not
	// contain information about the block hash.
	requests   *list.List
	requestNum uint64

	// oldestBlock stores the first block the sensor has seen so when fetching
	// parent blocks, it does not request blocks older than this.
	oldestBlock *types.Header
}

// Read reads an eth66 packet from the connection.
func (c *Conn) Read() Message {
	code, rawData, _, err := c.Conn.Read()
	if err != nil {
		return errorf("could not read from connection: %v", err)
	}

	var msg Message
	switch int(code) {
	case (Hello{}).Code():
		msg = new(Hello)
	case (Ping{}).Code():
		msg = new(Ping)
	case (Pong{}).Code():
		msg = new(Pong)
	case (Disconnect{}).Code():
		// Because disconnects have different formats, check the slice of
		// disconnects first then try the other.
		msg = new(Disconnects)
		if err := rlp.DecodeBytes(rawData, msg); err != nil {
			msg = new(Disconnect)
		}
	case (Status{}).Code():
		msg = new(Status)
	case (GetBlockHeaders{}).Code():
		ethMsg := new(eth.GetBlockHeadersPacket66)
		if err := rlp.DecodeBytes(rawData, ethMsg); err != nil {
			return errorf("could not rlp decode message: %v", err)
		}
		return (*GetBlockHeaders)(ethMsg)
	case (BlockHeaders{}).Code():
		ethMsg := new(eth.BlockHeadersPacket66)
		if err := rlp.DecodeBytes(rawData, ethMsg); err != nil {
			return errorf("could not rlp decode message: %v", err)
		}
		return (*BlockHeaders)(ethMsg)
	case (GetBlockBodies{}).Code():
		ethMsg := new(eth.GetBlockBodiesPacket66)
		if err := rlp.DecodeBytes(rawData, ethMsg); err != nil {
			return errorf("could not rlp decode message: %v", err)
		}
		return (*GetBlockBodies)(ethMsg)
	case (BlockBodies{}).Code():
		ethMsg := new(eth.BlockBodiesPacket66)
		if err := rlp.DecodeBytes(rawData, ethMsg); err != nil {
			return errorf("could not rlp decode message: %v", err)
		}
		return (*BlockBodies)(ethMsg)
	case (NewBlock{}).Code():
		msg = new(NewBlock)
	case (NewBlockHashes{}).Code():
		msg = new(NewBlockHashes)
	case (Transactions{}).Code():
		msg = new(Transactions)
	case (NewPooledTransactionHashes66{}).Code():
		// Try decoding to eth68
		ethMsg := new(NewPooledTransactionHashes)
		if err := rlp.DecodeBytes(rawData, ethMsg); err == nil {
			return ethMsg
		}
		msg = new(NewPooledTransactionHashes66)
	case (GetPooledTransactions{}.Code()):
		ethMsg := new(eth.GetPooledTransactionsPacket66)
		if err := rlp.DecodeBytes(rawData, ethMsg); err != nil {
			return errorf("could not rlp decode message: %v", err)
		}
		return (*GetPooledTransactions)(ethMsg)
	case (PooledTransactions{}.Code()):
		ethMsg := new(eth.PooledTransactionsPacket66)
		if err := rlp.DecodeBytes(rawData, ethMsg); err != nil {
			return errorf("could not rlp decode message: %v", err)
		}
		return (*PooledTransactions)(ethMsg)
	default:
		msg = errorf("invalid message code: %d", code)
	}

	if msg != nil {
		if err := rlp.DecodeBytes(rawData, msg); err != nil {
			return errorf("could not rlp decode message: %v", err)
		}
		return msg
	}
	return errorf("invalid message: %s", string(rawData))
}

// Write writes a eth packet to the connection.
func (c *Conn) Write(msg Message) error {
	payload, err := rlp.EncodeToBytes(msg)
	if err != nil {
		return err
	}
	_, err = c.Conn.Write(uint64(msg.Code()), payload)
	return err
}

// ReadSnap reads a snap/1 response with the given id from the connection.
func (c *Conn) ReadSnap(id uint64) (Message, error) {
	respId := id + 1
	start := time.Now()
	for respId != id && time.Since(start) < timeout {
		code, rawData, _, err := c.Conn.Read()
		if err != nil {
			return nil, fmt.Errorf("could not read from connection: %v", err)
		}
		var snpMsg interface{}
		switch int(code) {
		case (GetAccountRange{}).Code():
			snpMsg = new(GetAccountRange)
		case (AccountRange{}).Code():
			snpMsg = new(AccountRange)
		case (GetStorageRanges{}).Code():
			snpMsg = new(GetStorageRanges)
		case (StorageRanges{}).Code():
			snpMsg = new(StorageRanges)
		case (GetByteCodes{}).Code():
			snpMsg = new(GetByteCodes)
		case (ByteCodes{}).Code():
			snpMsg = new(ByteCodes)
		case (GetTrieNodes{}).Code():
			snpMsg = new(GetTrieNodes)
		case (TrieNodes{}).Code():
			snpMsg = new(TrieNodes)
		default:
			//return nil, fmt.Errorf("invalid message code: %d", code)
			continue
		}
		if err := rlp.DecodeBytes(rawData, snpMsg); err != nil {
			return nil, fmt.Errorf("could not rlp decode message: %v", err)
		}
		return snpMsg.(Message), nil
	}
	return nil, fmt.Errorf("request timed out")
}

// GetAccountRange represents an account range query.
type GetAccountRange snap.GetAccountRangePacket

func (msg GetAccountRange) Code() int     { return 33 }
func (msg GetAccountRange) ReqID() uint64 { return msg.ID }

type AccountRange snap.AccountRangePacket

func (msg AccountRange) Code() int     { return 34 }
func (msg AccountRange) ReqID() uint64 { return msg.ID }

type GetStorageRanges snap.GetStorageRangesPacket

func (msg GetStorageRanges) Code() int     { return 35 }
func (msg GetStorageRanges) ReqID() uint64 { return msg.ID }

type StorageRanges snap.StorageRangesPacket

func (msg StorageRanges) Code() int     { return 36 }
func (msg StorageRanges) ReqID() uint64 { return msg.ID }

type GetByteCodes snap.GetByteCodesPacket

func (msg GetByteCodes) Code() int     { return 37 }
func (msg GetByteCodes) ReqID() uint64 { return msg.ID }

type ByteCodes snap.ByteCodesPacket

func (msg ByteCodes) Code() int     { return 38 }
func (msg ByteCodes) ReqID() uint64 { return msg.ID }

type GetTrieNodes snap.GetTrieNodesPacket

func (msg GetTrieNodes) Code() int     { return 39 }
func (msg GetTrieNodes) ReqID() uint64 { return msg.ID }

type TrieNodes snap.TrieNodesPacket

func (msg TrieNodes) Code() int     { return 40 }
func (msg TrieNodes) ReqID() uint64 { return msg.ID }
