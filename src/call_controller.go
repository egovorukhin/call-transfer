package src

import (
	sippy_types "github.com/egovorukhin/go-b2bua/sippy/types"
	"sync"

	"github.com/egovorukhin/go-b2bua/sippy"
)

type callController struct {
	uaA                  sippy_types.UA
	uaO                  sippy_types.UA
	lock                 *sync.Mutex // this must be a reference to prevent memory leak
	id                   int64
	cmap                 *CallMap
	evTry                *sippy.CCEventTry
	transferIsInProgress bool
}

func NewCallController(cmap *CallMap) *callController {
	s := &callController{
		id:                   <-cmap.NextCcId,
		uaO:                  nil,
		lock:                 new(sync.Mutex),
		cmap:                 cmap,
		transferIsInProgress: false,
	}
	s.uaA = sippy.NewUA(cmap.SipTm, cmap.config, cmap.config.NhAddr, s, s.lock, nil)
	s.uaA.SetDeadCb(s.aDead)
	//s.uaA.SetCreditTime(5 * time.Second)
	return s
}

func (s *callController) handleTransfer(event sippy_types.CCEvent, ua sippy_types.UA) {
	switch ua {
	case s.uaA:
		if _, ok := event.(*sippy.CCEventConnect); ok {
			// Transfer is completed.
			s.transferIsInProgress = false
		}
		s.uaO.RecvEvent(event)
	case s.uaO:
		if _, ok := event.(*sippy.CCEventPreConnect); ok {
			//
			// Convert into CCEventUpdate.
			//
			// Here 200 OK response from the new callee has been received
			// and now re-INVITE will be sent to the caller.
			//
			// The CCEventPreConnect is here because the outgoing call to the
			// new destination has been sent using the late offer model, i.e.
			// the outgoing INVITE was body-less.
			//
			event = sippy.NewCCEventUpdate(event.GetRtime(), event.GetOrigin(), event.GetReason(),
				event.GetMaxForwards(), event.GetBody().GetCopy())
		}
		s.uaA.RecvEvent(event)
	}
}

func (s *callController) RecvEvent(event sippy_types.CCEvent, ua sippy_types.UA) {
	if s.transferIsInProgress {
		s.handleTransfer(event, ua)
		return
	}
	if ua == s.uaA {
		if s.uaO == nil {
			ev_try, ok := event.(*sippy.CCEventTry)
			if !ok {
				// Some weird event received
				s.uaA.RecvEvent(sippy.NewCCEventDisconnect(nil, event.GetRtime(), ""))
				return
			}
			s.uaO = sippy.NewUA(s.cmap.SipTm, s.cmap.config, s.cmap.config.NhAddr, s, s.lock, nil)
			s.uaO.SetRAddr(s.cmap.config.NhAddr)
			s.evTry = ev_try
		}
		s.uaO.RecvEvent(event)
	} else {
		if ev_disc, ok := event.(*sippy.CCEventDisconnect); ok {
			redirect_url := ev_disc.GetRedirectURL()
			if redirect_url != nil {
				//
				// Either REFER or a BYE with Also: has been received from the callee.
				//
				// Do not interrupt the caller call leg and create a new call leg
				// to the new destination.
				//
				cld := redirect_url.GetUrl().Username

				//nh_addr := &sippy_net.HostPort{ redirect_url.GetUrl().Host, redirect_url.GetUrl().Port }
				nh_addr := s.cmap.config.NhAddr

				s.uaO = sippy.NewUA(s.cmap.SipTm, s.cmap.config, nh_addr, s, s.lock, nil)
				ev_try, _ := sippy.NewCCEventTry(s.evTry.GetSipCallId(),
					s.evTry.GetCLI(), cld, nil /*body*/, nil /*auth*/, s.evTry.GetCallerName(),
					ev_disc.GetRtime(), s.evTry.GetOrigin())
				s.transferIsInProgress = true
				s.uaO.RecvEvent(ev_try)
				return
			}
		}
		s.uaA.RecvEvent(event)
	}
}

func (s *callController) aDead() {
	s.cmap.Remove(s.id)
}

func (s *callController) Shutdown() {
	s.uaA.Disconnect(nil, "")
}

func (s *callController) String() string {
	res := "uaA:" + s.uaA.String() + ", uaO: "
	if s.uaO == nil {
		res += "nil"
	} else {
		res += s.uaO.String()
	}
	return res
}
