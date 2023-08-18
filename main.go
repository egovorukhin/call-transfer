package main

import (
	"call-transfer/src"
	crand "crypto/rand"
	"flag"
	"github.com/egovorukhin/go-b2bua/sippy"
	sippy_conf "github.com/egovorukhin/go-b2bua/sippy/conf"
	sippy_log "github.com/egovorukhin/go-b2bua/sippy/log"
	sippy_net "github.com/egovorukhin/go-b2bua/sippy/net"
	mrand "math/rand"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var Next_cc_id chan int64

func init() {
	Next_cc_id = make(chan int64)
	go func() {
		var id int64 = 1
		for {
			Next_cc_id <- id
			id++
		}
	}()
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	buf := make([]byte, 8)
	_, _ = crand.Read(buf)
	var salt int64
	for _, c := range buf {
		salt = (salt << 8) | int64(c)
	}
	mrand.New(mrand.NewSource(salt))

	var lAddr, nhAddr, logfile string
	var lPort int
	var foreground bool

	flag.StringVar(&lAddr, "l", "", "Local addr")
	flag.IntVar(&lPort, "p", -1, "Local port")
	flag.StringVar(&nhAddr, "n", "", "Next hop address")
	flag.BoolVar(&foreground, "f", false, "Run in foreground")
	flag.StringVar(&logfile, "L", "/var/log/sip.log", "Log file")
	flag.Parse()

	errorLogger := sippy_log.NewErrorLogger()
	sipLogger, err := sippy_log.NewSipLogger("b2bua", logfile)
	if err != nil {
		errorLogger.Error(err)
		return
	}
	config := &src.MyConfig{
		Config: sippy_conf.NewConfig(errorLogger, sipLogger),
		NhAddr: sippy_net.NewHostPort("192.168.0.102", "5060"), // next hop address
	}
	//config.SetIPV6Enabled(false)
	if nhAddr != "" {
		var parts []string
		var addr string

		if strings.HasPrefix(nhAddr, "[") {
			parts = strings.SplitN(nhAddr, "]", 2)
			addr = parts[0] + "]"
			if len(parts) == 2 {
				parts = strings.SplitN(parts[1], ":", 2)
			}
		} else {
			parts = strings.SplitN(nhAddr, ":", 2)
			addr = parts[0]
		}
		port := "5060"
		if len(parts) == 2 {
			port = parts[1]
		}
		config.NhAddr = sippy_net.NewHostPort(addr, port)
	}
	config.SetMyUAName("Sippy B2BUA (Simple)")
	config.SetAllowFormats([]int{0, 8, 18, 100, 101})
	if lAddr != "" {
		config.SetMyAddress(sippy_net.NewMyAddress(lAddr))
	}
	config.SetSipAddress(config.GetMyAddress())
	if lPort > 0 {
		config.SetMyPort(sippy_net.NewMyPort(strconv.Itoa(lPort)))
	}
	config.SetSipPort(config.GetMyPort())
	cmap := src.NewCallMap(config, errorLogger, Next_cc_id)
	sip_tm, err := sippy.NewSipTransactionManager(config, cmap)
	if err != nil {
		errorLogger.Error(err)
		return
	}
	cmap.SipTm = sip_tm
	cmap.Proxy = sippy.NewStatefulProxy(sip_tm, config.NhAddr, config)
	go sip_tm.Run()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGTERM, syscall.SIGINT)
	signal.Ignore(syscall.SIGHUP, syscall.SIGPIPE)
	select {
	case <-signalChan:
		cmap.Shutdown()
		sip_tm.Shutdown()
		time.Sleep(time.Second)
		break
	}
}
