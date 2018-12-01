package eeproxy

import (
	"github.com/icon-project/goloop/common"
	"github.com/icon-project/goloop/common/codec"
	"github.com/icon-project/goloop/common/ipc"
	"github.com/icon-project/goloop/module"
	"github.com/pkg/errors"
	"log"
	"math/big"
	"sync"
)

type Message uint

const (
	msgVESION   uint = 0
	msgINVOKE        = 1
	msgRESULT        = 2
	msgGETVALUE      = 3
	msgSETVALUE      = 4
	msgCALL          = 5
	msgEVENT         = 6
	msgGETINFO       = 7
)

type CallContext interface {
	GetValue(key []byte) ([]byte, error)
	SetValue(key, value []byte) error
	GetInfo() map[string]interface{}
	OnEvent(idxcnt uint16, msgs [][]byte)
	OnResult(status uint16, steps *big.Int, result []byte)
	OnCall(from, to module.Address, value, limit *big.Int, params []byte)
}

type Proxy interface {
	Invoke(ctx CallContext, code string, from, to module.Address,
		value, limit *big.Int, method string, params []byte) error
	SendResult(ctx CallContext, status uint16, steps *big.Int, result []byte) error
	Release()
}

type callFrame struct {
	addr module.Address
	ctx  CallContext

	prev *callFrame
}

type proxy struct {
	lock     sync.Mutex
	reserved bool
	mgr      *manager

	conn ipc.Connection

	version   uint16
	pid       uint32
	scoreType scoreType

	frame *callFrame

	next  *proxy
	pprev **proxy
}

type versionMessage struct {
	Version uint16 `codec:"version"`
	PID     uint32 `codec:"pid"`
	Type    string
}

type invokeMessage struct {
	Code   string         `codec:"code"`
	From   common.Address `codec:"from"`
	To     common.Address `codec:"to"`
	Value  common.HexInt  `codec:"value"`
	Limit  common.HexInt  `codec:"limit"`
	Method string         `codec:"method"`
	Params []byte         `codec:"params"`
}

type setMessage struct {
	Key   []byte `codec:"key"`
	Value []byte `codec:"value"`
}

type callMessage struct {
	To     common.Address
	Value  common.HexInt
	Limit  common.HexInt
	Method string
	Params []byte
}

type eventMessage struct {
	Index    uint16
	Messages [][]byte
}

func (p *proxy) Invoke(ctx CallContext, code string, from, to module.Address,
	value, limit *big.Int, method string, params []byte,
) error {
	var m invokeMessage
	m.Code = code
	m.From.SetBytes(from.Bytes())
	m.To.SetBytes(to.Bytes())
	m.Value.Set(value)
	m.Limit.Set(limit)
	m.Method = method
	m.Params = params

	p.lock.Lock()
	defer p.lock.Unlock()
	p.frame = &callFrame{
		addr: to,
		ctx:  ctx,
		prev: p.frame,
	}
	return p.conn.Send(msgINVOKE, &m)
}

type resultMessage struct {
	Status   uint16
	StepUsed common.HexInt
	Result   []byte
}

func (p *proxy) reserve() bool {
	p.lock.Lock()
	defer p.lock.Unlock()

	if p.reserved {
		return false
	}
	return true
}

func (p *proxy) Release() {
	p.lock.Lock()
	if !p.reserved {
		p.lock.Unlock()
		return
	}
	p.reserved = false
	if p.frame == nil {
		p.lock.Unlock()
		p.mgr.onReady(p.scoreType, p)
	}
}

func (p *proxy) SendResult(ctx CallContext, status uint16, stepUsed *big.Int, result []byte) error {
	var m resultMessage
	m.Status = status
	m.StepUsed.Set(stepUsed)
	m.Result = result
	return p.conn.Send(msgRESULT, &m)
}

func (p *proxy) HandleMessage(c ipc.Connection, msg uint, data []byte) error {
	switch msg {
	case msgVESION:
		var m versionMessage
		if _, err := codec.MP.UnmarshalFromBytes(data, &m); err != nil {
			c.Close()
			return err
		}
		log.Printf("VERSION:%d, PID:%d", m.Version, m.PID)
		p.version = m.Version
		p.pid = m.PID
		if t, ok := scoreNameToType[m.Type]; !ok {
			return errors.Errorf("UnknownSCOREName(%s)", m.Type)
		} else {
			p.scoreType = t
		}

		p.mgr.onReady(p.scoreType, p)
		return nil

	case msgCALL:
		var m callMessage
		if _, err := codec.MP.UnmarshalFromBytes(data, &m); err != nil {
			c.Close()
			return err
		}
		p.frame.ctx.OnCall(p.frame.addr,
			&m.To, &m.Value.Int, &m.Limit.Int, m.Params)
		return nil

	case msgRESULT:
		var m resultMessage
		if _, err := codec.MP.UnmarshalFromBytes(data, &m); err != nil {
			c.Close()
			return err
		}
		p.lock.Lock()
		frame := p.frame
		p.frame = frame.prev
		p.lock.Unlock()

		frame.ctx.OnResult(m.Status, &m.StepUsed.Int, m.Result)

		p.lock.Lock()
		if p.frame == nil && !p.reserved {
			p.lock.Unlock()
			p.mgr.onReady(p.scoreType, p)
		} else {
			p.lock.Unlock()
		}
		return nil

	case msgGETVALUE:
		var m []byte
		if _, err := codec.MP.UnmarshalFromBytes(data, &m); err != nil {
			c.Close()
			return err
		}
		value, err := p.frame.ctx.GetValue(m)
		if err != nil || value == nil {
			value = []byte{}
		}
		return p.conn.Send(msgGETVALUE, value)

	case msgSETVALUE:
		var m setMessage
		if _, err := codec.MP.UnmarshalFromBytes(data, &m); err != nil {
			c.Close()
			return err
		}
		return p.frame.ctx.SetValue(m.Key, m.Value)

	case msgEVENT:
		var m eventMessage
		if _, err := codec.MP.UnmarshalFromBytes(data, &m); err != nil {
			c.Close()
			return err
		}
		p.frame.ctx.OnEvent(m.Index, m.Messages)
		return nil

	case msgGETINFO:
		v := p.frame.ctx.GetInfo()
		eo, err := common.EncodeAny(v)
		if err != nil {
			return err
		}
		return p.conn.Send(msgGETINFO, eo)

	default:
		return errors.Errorf("UnknownMessage(%d)", msg)
	}
}

func (p *proxy) HandleMessages() error {
	for {
		err := p.conn.HandleMessage()
		if err != nil {
			log.Printf("Error on conn.HandleMessage() err=%+v\n", err)
			break
		}
	}
	p.mgr.detach(p)
	p.conn.Close()
	return nil
}

func newConnection(m *manager, c ipc.Connection) (*proxy, error) {
	p := &proxy{
		mgr:  m,
		conn: c,
	}
	c.SetHandler(msgVESION, p)
	c.SetHandler(msgRESULT, p)
	c.SetHandler(msgGETVALUE, p)
	c.SetHandler(msgSETVALUE, p)
	c.SetHandler(msgCALL, p)
	c.SetHandler(msgGETINFO, p)
	return p, nil
}
