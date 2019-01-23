package network

import (
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/icon-project/goloop/common/crypto"
	"github.com/icon-project/goloop/module"
)

//Negotiation map<channel, map<protocolHandler.name, {protocol, []subProtocol}>>
type ChannelNegotiator struct {
	*peerHandler
	netAddress NetAddress
}

func newChannelNegotiator(netAddress NetAddress) *ChannelNegotiator {
	cn := &ChannelNegotiator{
		netAddress:  netAddress,
		peerHandler: newPeerHandler(newLogger("ChannelNegotiator", "")),
	}
	return cn
}

func (cn *ChannelNegotiator) onPeer(p *Peer) {
	cn.log.Println("onPeer", p)
	if !p.incomming {
		cn.sendJoinRequest(p)
	}
}

func (cn *ChannelNegotiator) onError(err error, p *Peer, pkt *Packet) {
	cn.log.Println("onError", err, p, pkt)
	cn.peerHandler.onError(err, p, pkt)
}

func (cn *ChannelNegotiator) onPacket(pkt *Packet, p *Peer) {
	cn.log.Println("onPacket", pkt, p)
	switch pkt.protocol {
	case PROTO_CONTOL:
		switch pkt.subProtocol {
		case PROTO_CHAN_JOIN_REQ:
			cn.handleJoinRequest(pkt, p)
		case PROTO_CHAN_JOIN_RESP:
			cn.handleJoinResponse(pkt, p)
		}
	}
}

type JoinRequest struct {
	Channel string
	Addr    NetAddress
}

type JoinResponse struct {
	Channel string
	Addr    NetAddress
}

func (cn *ChannelNegotiator) sendJoinRequest(p *Peer) {
	m := &JoinRequest{Channel: p.channel, Addr: cn.netAddress}
	cn.sendMessage(PROTO_CHAN_JOIN_REQ, m, p)
	cn.log.Println("sendJoinRequest", m, p)
}

func (cn *ChannelNegotiator) handleJoinRequest(pkt *Packet, p *Peer) {
	rm := &JoinRequest{}
	cn.decode(pkt.payload, rm)
	cn.log.Println("handleJoinRequest", rm, p)
	p.channel = rm.Channel
	p.netAddress = rm.Addr

	m := &JoinResponse{Channel: p.channel, Addr: cn.netAddress}
	cn.sendMessage(PROTO_CHAN_JOIN_RESP, m, p)

	cn.nextOnPeer(p)
}

func (cn *ChannelNegotiator) handleJoinResponse(pkt *Packet, p *Peer) {
	rm := &JoinResponse{}
	cn.decode(pkt.payload, rm)
	cn.log.Println("handleJoinResponse", rm, p)
	p.channel = rm.Channel
	p.netAddress = rm.Addr

	cn.nextOnPeer(p)
}

type Authenticator struct {
	*peerHandler
	wallet             module.Wallet
	secureSuites       map[string][]SecureSuite
	secureAeads        map[string][]SecureAeadSuite
	secureKeyNum       int
	mtx                sync.Mutex
}

func newAuthenticator(w module.Wallet) *Authenticator {
	_, err := crypto.ParsePublicKey(w.PublicKey())
	if err != nil {
		panic(err)
	}
	a := &Authenticator{
		wallet:             w,
		secureSuites:       make(map[string][]SecureSuite),
		secureAeads:        make(map[string][]SecureAeadSuite),
		secureKeyNum:       2,
		peerHandler:        newPeerHandler(newLogger("Authenticator", "")),
	}
	return a
}

//callback from PeerHandler.nextOnPeer
func (a *Authenticator) onPeer(p *Peer) {
	a.log.Println("onPeer", p)
	if !p.incomming {
		a.sendSecureRequest(p)
	}
}

//TODO callback from Peer.sendRoutine or Peer.receiveRoutine
func (a *Authenticator) onError(err error, p *Peer, pkt *Packet) {
	a.log.Println("onError", err, p, pkt)
	a.peerHandler.onError(err, p, pkt)
}

//callback from Peer.receiveRoutine
func (a *Authenticator) onPacket(pkt *Packet, p *Peer) {
	a.log.Println("onPacket", pkt, p)
	switch pkt.protocol {
	case PROTO_CONTOL:
		switch pkt.subProtocol {
		case PROTO_AUTH_KEY_REQ:
			a.handleSecureRequest(pkt, p)
		case PROTO_AUTH_KEY_RESP:
			a.handleSecureResponse(pkt, p)
		case PROTO_AUTH_SIGN_REQ:
			a.handleSignatureRequest(pkt, p)
		case PROTO_AUTH_SIGN_RESP:
			a.handleSignatureResponse(pkt, p)
		}
	default:
		//ignore
	}
}

func (a *Authenticator) Signature(content []byte) []byte {
	defer a.mtx.Unlock()
	a.mtx.Lock()
	h := crypto.SHA3Sum256(content)
	sb, _ := a.wallet.Sign(h)
	return sb
}

func (a *Authenticator) VerifySignature(publicKey []byte, signature []byte, content []byte) (module.PeerID, error) {
	pubKey, err := crypto.ParsePublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf("fail to parse public key : %s", err.Error())
	}
	id := NewPeerIDFromPublicKey(pubKey)
	if id == nil {
		return nil, fmt.Errorf("fail to create peer id by public key : %s", pubKey.String())
	}
	s, err := crypto.ParseSignature(signature)
	if err != nil {
		return nil, fmt.Errorf("fail to parse signature : %s", err.Error())
	}
	h := crypto.SHA3Sum256(content)
	if !s.Verify(h, pubKey) {
		err = errors.New("fail to verify signature")
	}
	return id, err
}

type SecureRequest struct {
	Channel          string
	SecureSuites     []SecureSuite
	SecureAeadSuites []SecureAeadSuite
	SecureParam      []byte
}
type SecureResponse struct {
	Channel         string
	SecureSuite     SecureSuite
	SecureAeadSuite SecureAeadSuite
	SecureParam     []byte
	SecureError     SecureError
}
type SignatureRequest struct {
	PublicKey []byte
	Signature []byte
	Rtt       time.Duration
}
type SignatureResponse struct {
	PublicKey []byte
	Signature []byte
	Rtt       time.Duration
	Error     string
}

func (a *Authenticator) sendSecureRequest(p *Peer) {
	p.secureKey = newSecureKey(DefaultSecureEllipticCurve, DefaultSecureKeyLogWriter)
	sms := a.secureSuites[p.channel]
	if len(sms) == 0 {
		sms = DefaultSecureSuites
	}
	sas := a.secureAeads[p.channel]
	if len(sas) == 0 {
		sas = DefaultSecureAeadSuites
	}
	m := &SecureRequest{
		Channel:          p.channel,
		SecureSuites:     sms,
		SecureAeadSuites: sas,
		SecureParam:      p.secureKey.marshalPublicKey(),
	}

	p.rtt.Start()
	a.sendMessage(PROTO_AUTH_KEY_REQ, m, p)
	a.log.Println("sendSecureRequest", m, p)
}

func (a *Authenticator) handleSecureRequest(pkt *Packet, p *Peer) {
	rm := &SecureRequest{}
	a.decode(pkt.payload, rm)
	a.log.Println("handleSecureRequest", rm, p)

	m := &SecureResponse{
		Channel:         p.channel,
		SecureSuite:     SecureSuiteUnknown,
		SecureAeadSuite: SecureAeadSuiteUnknown,
	}

	sms := a.secureSuites[rm.Channel]
	if len(sms) == 0 {
		sms = DefaultSecureSuites
	}
SecureSuiteLoop:
	for _, sm := range sms {
		for _, rsm := range rm.SecureSuites {
			if rsm == sm {
				m.SecureSuite = sm
				a.log.Println("handleSecureRequest", p.ConnString(), "SecureSuite", sm)
				break SecureSuiteLoop
			}
		}
	}
	if m.SecureSuite == SecureSuiteUnknown {
		m.SecureError = SecureErrorInvalid
	}

	sas := a.secureAeads[rm.Channel]
	if len(sas) == 0 {
		sas = DefaultSecureAeadSuites
	}
SecureAeadLoop:
	for _, sa := range sas {
		for _, rsa := range rm.SecureAeadSuites {
			if rsa == sa {
				m.SecureAeadSuite = sa
				a.log.Println("handleSecureRequest", p.ConnString(), "SecureAeadSuite", sa)
				break SecureAeadLoop
			}
		}
	}
	if m.SecureAeadSuite == SecureAeadSuiteUnknown && (m.SecureSuite == SecureSuiteEcdhe || m.SecureSuite == SecureSuiteTls) {
		m.SecureError = SecureErrorInvalid
	}

	switch p.conn.(type) {
	case *SecureConn:
		m.SecureSuite = SecureSuiteEcdhe
		m.SecureError = SecureErrorEstablished
	case *tls.Conn:
		m.SecureSuite = SecureSuiteTls
		m.SecureError = SecureErrorEstablished
	default:
		p.secureKey = newSecureKey(DefaultSecureEllipticCurve, DefaultSecureKeyLogWriter)
		m.SecureParam = p.secureKey.marshalPublicKey()
	}

	p.rtt.Start()
	a.sendMessage(PROTO_AUTH_KEY_RESP, m, p)
	if m.SecureError != SecureErrorNone {
		err := fmt.Errorf("handleSecureRequest error[%v]", m.SecureError)
		a.log.Println("Warning", "handleSecureRequest", p.ConnString(), "SecureError", err)
		p.CloseByError(err)
		return
	}

	p.channel = rm.Channel
	err := p.secureKey.setup(m.SecureAeadSuite, rm.SecureParam, p.incomming, a.secureKeyNum)
	if err != nil {
		a.log.Println("Warning", "handleSecureRequest", p.ConnString(), "failed secureKey.setup", err)
		p.CloseByError(err)
		return
	}
	switch m.SecureSuite {
	case SecureSuiteEcdhe:
		secureConn, err := NewSecureConn(p.conn, m.SecureAeadSuite, p.secureKey)
		if err != nil {
			a.log.Println("Warning", "handleSecureRequest", p.ConnString(), "failed NewSecureConn", err)
			p.CloseByError(err)
			return
		}
		p.ResetConn(secureConn)
	case SecureSuiteTls:
		config, err := p.secureKey.tlsConfig()
		if err != nil {
			a.log.Println("Warning", "handleSecureRequest", p.ConnString(), "failed tlsConfig", err)
			p.CloseByError(err)
			return
		}
		tlsConn := tls.Server(p.conn, config)
		p.ResetConn(tlsConn)
	}
}

func (a *Authenticator) handleSecureResponse(pkt *Packet, p *Peer) {
	rm := &SecureResponse{}
	a.decode(pkt.payload, rm)
	a.log.Println("handleSecureResponse", rm, p)
	p.rtt.Stop()

	if rm.SecureError != SecureErrorNone {
		err := fmt.Errorf("handleSecureResponse error[%v]", rm.SecureError)
		a.log.Println("Warning", "handleSecureResponse", p.ConnString(), "SecureError", err)
		p.CloseByError(err)
		return
	}

	var rsm SecureSuite = SecureSuiteUnknown
	sms := a.secureSuites[p.channel]
	if len(sms) == 0 {
		sms = DefaultSecureSuites
	}
SecureSuiteLoop:
	for _, sm := range sms {
		if sm == rm.SecureSuite {
			rsm = sm
			break SecureSuiteLoop
		}
	}
	if rsm == SecureSuiteUnknown {
		err := fmt.Errorf("handleSecureResponse invalid SecureSuite %d", rm.SecureSuite)
		a.log.Println("Warning", "handleSecureResponse", p.ConnString(), "SecureError", err)
		p.CloseByError(err)
		return
	}

	var rsa SecureAeadSuite = SecureAeadSuiteUnknown
	sas := a.secureAeads[rm.Channel]
	if len(sas) == 0 {
		sas = DefaultSecureAeadSuites
	}
SecureAeadLoop:
	for _, sa := range sas {
		if sa == rm.SecureAeadSuite {
			rsa = sa
			break SecureAeadLoop
		}
	}
	if rsa == SecureAeadSuiteUnknown && (rsm == SecureSuiteEcdhe || rsm == SecureSuiteTls) {
		err := fmt.Errorf("handleSecureResponse invalid SecureSuite %d SecureAeadSuite %d", rm.SecureSuite, rm.SecureAeadSuite)
		a.log.Println("Warning", "handleSecureResponse", p.ConnString(), "SecureError", err)
		p.CloseByError(err)
		return
	}

	var secured bool
	switch p.conn.(type) {
	case *SecureConn:
		secured = true
	case *tls.Conn:
		secured = true
	}
	if secured {
		err := fmt.Errorf("handleSecureResponse already established secure connection %T", p.conn)
		a.log.Println("Warning", "handleSecureResponse", p.ConnString(), "SecureError", err)
		p.CloseByError(err)
		return
	}

	err := p.secureKey.setup(rm.SecureAeadSuite, rm.SecureParam, p.incomming, a.secureKeyNum)
	if err != nil {
		a.log.Println("Warning", "handleSecureRequest", p.ConnString(), "failed secureKey.setup", err)
		p.CloseByError(err)
		return
	}
	switch rm.SecureSuite {
	case SecureSuiteEcdhe:
		secureConn, err := NewSecureConn(p.conn, rm.SecureAeadSuite, p.secureKey)
		if err != nil {
			a.log.Println("Warning", "handleSecureResponse", p.ConnString(), "failed NewSecureConn", err)
			p.CloseByError(err)
			return
		}
		p.ResetConn(secureConn)
	case SecureSuiteTls:
		config, err := p.secureKey.tlsConfig()
		if err != nil {
			a.log.Println("Warning", "handleSecureResponse", p.ConnString(), "failed tlsConfig", err)
			p.CloseByError(err)
			return
		}
		tlsConn := tls.Client(p.conn, config)
		if err := tlsConn.Handshake(); err != nil {
			a.log.Println("Warning", "handleSecureResponse", p.ConnString(), "failed tls handshake", err)
			p.CloseByError(err)
			return
		}
		p.ResetConn(tlsConn)
	}

	m := &SignatureRequest{
		PublicKey: a.wallet.PublicKey(),
		Signature: a.Signature(p.secureKey.extra),
		Rtt:       p.rtt.last,
	}
	a.sendMessage(PROTO_AUTH_SIGN_REQ, m, p)
}

func (a *Authenticator) handleSignatureRequest(pkt *Packet, p *Peer) {
	rm := &SignatureRequest{}
	a.decode(pkt.payload, rm)
	a.log.Println("handleSignatureRequest", rm, p)
	p.rtt.Stop()
	df := rm.Rtt - p.rtt.last
	if df > DefaultRttAccuracy {
		a.log.Println("Warning", "handleSignatureRequest", df, "DefaultRttAccuracy", DefaultRttAccuracy)
	}

	m := &SignatureResponse{
		PublicKey: a.wallet.PublicKey(),
		Signature: a.Signature(p.secureKey.extra),
		Rtt:       p.rtt.last,
	}

	id, err := a.VerifySignature(rm.PublicKey, rm.Signature, p.secureKey.extra)
	if err != nil {
		m = &SignatureResponse{Error: err.Error()}
	}
	p.id = id
	a.sendMessage(PROTO_AUTH_SIGN_RESP, m, p)

	if m.Error != "" {
		err := fmt.Errorf("handleSignatureRequest error[%v]", m.Error)
		a.log.Println("Warning", "handleSignatureRequest", p.ConnString(), "Error", err)
		p.CloseByError(err)
		return
	}
	a.nextOnPeer(p)
}

func (a *Authenticator) handleSignatureResponse(pkt *Packet, p *Peer) {
	rm := &SignatureResponse{}
	a.decode(pkt.payload, rm)
	a.log.Println("handleSignatureResponse", rm, p)

	df := rm.Rtt - p.rtt.last
	if df > DefaultRttAccuracy {
		a.log.Println("Warning", "handleSignatureResponse", df, "DefaultRttAccuracy", DefaultRttAccuracy)
	}

	if rm.Error != "" {
		err := fmt.Errorf("handleSignatureResponse error[%v]", rm.Error)
		a.log.Println("Warning", "handleSignatureResponse", p.ConnString(), "Error", err)
		p.CloseByError(err)
		return
	}

	id, err := a.VerifySignature(rm.PublicKey, rm.Signature, p.secureKey.extra)
	if err != nil {
		err := fmt.Errorf("handleSignatureResponse error[%v]", err)
		a.log.Println("Warning", "handleSignatureResponse", p.ConnString(), "Error", err)
		p.CloseByError(err)
		return
	}
	p.id = id
	if !p.id.Equal(pkt.src) {
		a.log.Println("Warning", "handleSignatureResponse", "id doesnt match pkt:", pkt.src, ",expected:", p.id)
	}
	a.nextOnPeer(p)
}
