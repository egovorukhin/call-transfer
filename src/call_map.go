package src

import (
	"sync"

	"github.com/egovorukhin/go-b2bua/sippy/conf"
	"github.com/egovorukhin/go-b2bua/sippy/log"
	"github.com/egovorukhin/go-b2bua/sippy/net"
	"github.com/egovorukhin/go-b2bua/sippy/types"
)

type CallMap struct {
	config    *MyConfig
	logger    sippy_log.ErrorLogger
	SipTm     sippy_types.SipTransactionManager
	Proxy     sippy_types.StatefulProxy
	ccmap     map[int64]*callController
	ccmapLock sync.Mutex
	NextCcId  chan int64
}

type MyConfig struct {
	sippy_conf.Config
	NhAddr *sippy_net.HostPort
}

func NewCallMap(config *MyConfig, logger sippy_log.ErrorLogger, NextCcId chan int64) *CallMap {
	return &CallMap{
		logger:   logger,
		config:   config,
		ccmap:    make(map[int64]*callController),
		NextCcId: NextCcId,
	}
}

func (s *CallMap) OnNewDialog(req sippy_types.SipRequest, tr sippy_types.ServerTransaction) (sippy_types.UA, sippy_types.RequestReceiver, sippy_types.SipResponse) {
	toBody, err := req.GetTo().GetBody(s.config)
	if err != nil {
		s.logger.Error("CallMap::OnNewDialog: #1: " + err.Error())
		return nil, nil, req.GenResponse(500, "Internal Server Error", nil, nil)
	}
	if toBody.GetTag() != "" {
		// Request within dialog, but no such dialog
		return nil, nil, req.GenResponse(481, "Call Leg/Transaction Does Not Exist", nil, nil)
	}
	if req.GetMethod() == "INVITE" {
		// New dialog
		cc := NewCallController(s)
		s.ccmapLock.Lock()
		s.ccmap[cc.id] = cc
		s.ccmapLock.Unlock()
		return cc.uaA, cc.uaA, nil
	}
	if req.GetMethod() == "REGISTER" {
		// Registration
		return nil, s.Proxy, nil
	}
	if req.GetMethod() == "NOTIFY" || req.GetMethod() == "PING" {
		// Whynot?
		return nil, nil, req.GenResponse(200, "OK", nil, nil)
	}
	return nil, nil, req.GenResponse(501, "Not Implemented", nil, nil)
}

func (s *CallMap) Remove(ccid int64) {
	s.ccmapLock.Lock()
	defer s.ccmapLock.Unlock()
	delete(s.ccmap, ccid)
}

func (s *CallMap) Shutdown() {
	acalls := []*callController{}
	s.ccmapLock.Lock()
	for _, cc := range s.ccmap {
		//println(cc.String())
		acalls = append(acalls, cc)
	}
	s.ccmapLock.Unlock()
	for _, cc := range acalls {
		cc.Shutdown()
	}
}
