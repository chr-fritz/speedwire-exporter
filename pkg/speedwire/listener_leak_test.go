package speedwire

import (
	"context"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chr-fritz/speedwire-exporter/pkg/collector"
	"github.com/chr-fritz/speedwire-exporter/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/bboehmke/sunny"
	"gitlab.com/bboehmke/sunny/proto"
	"golang.org/x/net/ipv4"
)

// recvCountingLogger counts sunny's "recv" trace lines so the test can confirm the listener's
// connection actually received the fake broadcaster's packets (i.e. the discovery path was
// genuinely exercised), independent of how many goroutines piled up.
type recvCountingLogger struct{ recv int64 }

func (l *recvCountingLogger) Printf(format string, _ ...interface{}) {
	if strings.HasPrefix(format, "recv ") {
		atomic.AddInt64(&l.recv, 1)
	}
}

var (
	leakTestOnce   sync.Once
	leakTestLogger *recvCountingLogger
)

// setupLeakTestGlobals shortens discovery so a single window happens well inside the test and
// installs a logger that counts received packets. It writes those package globals exactly
// once and never restores them.
//
// That is deliberate. This test intentionally leaves a discovery window's worth of NewDevice
// goroutines draining - each retries for 3s against a source that never completes the
// handshake - so they keep reading discoveryWindow/discoveryInterval (in discoverLoop) and
// sunny.Log (on every packet sent or received) for minutes after the test returns. All of
// these are plain, unsynchronised package variables, so restoring them afterwards, or setting
// them again on a repeat run (go test -count=2), is a data race that -race reports. Waiting
// for the drain instead would add ~10 minutes to the suite. Writing them once is harmless:
// this is the only test that runs a Listener, and recvCountingLogger merely counts "recv "
// lines.
//
// The rule this encodes also holds in production: sunny.Log must be written before any
// listener runs and never again, which is what InstallSunnyLogger does at startup.
func setupLeakTestGlobals() *recvCountingLogger {
	leakTestOnce.Do(func() {
		discoveryWindow = 400 * time.Millisecond
		discoveryInterval = time.Hour

		leakTestLogger = &recvCountingLogger{}
		sunny.Log = leakTestLogger
		sunny.EnableDetailedPacketLogging(false)
	})
	return leakTestLogger
}

// TestListenerDiscoveryDoesNotLeakGoroutines guards against unbounded goroutine growth caused by
// running sunny's DiscoverDevices continuously. A device that keeps multicasting valid Speedwire
// packets whose NewDevice handshake never completes made the listener spawn (and pile up) a
// goroutine per received packet forever. With bounded, periodic discovery the goroutine count
// must plateau once a discovery window closes instead of climbing monotonically.
func TestListenerDiscoveryDoesNotLeakGoroutines(t *testing.T) {
	const iface = "lo0"
	group := &net.UDPAddr{IP: net.ParseIP("239.12.255.254"), Port: 9522}

	ifi, err := net.InterfaceByName(iface)
	if err != nil {
		t.Skipf("no %s: %v", iface, err)
	}

	logger := setupLeakTestGlobals()

	// Control receiver: joining the group on lo0 is what makes the loopback multicast reliably
	// deliver to other local group members (including the listener's socket) on some platforms.
	ctrl, err := net.ListenMulticastUDP("udp4", ifi, group)
	if err != nil {
		t.Skipf("control listen: %v", err)
	}
	defer func() { _ = ctrl.Close() }()
	go func() {
		buf := make([]byte, 2048)
		for {
			if _, _, err := ctrl.ReadFromUDP(buf); err != nil {
				return
			}
		}
	}()

	// Valid packet (>=20 bytes, parses) with no SmaNet2 entry, so NewDevice never completes.
	var pkt proto.Packet
	pkt.AddEntry(&proto.GroupPacketEntry{Group: 0x00000001})
	pkt.AddEntry(&proto.GroupPacketEntry{Group: 0x00000001})
	payload := pkt.Bytes()

	sendConn, err := net.ListenPacket("udp4", "0.0.0.0:0")
	require.NoError(t, err)
	defer func() { _ = sendConn.Close() }()
	pc := ipv4.NewPacketConn(sendConn)
	if err := pc.SetMulticastInterface(ifi); err != nil {
		t.Skipf("cannot set multicast interface: %v", err)
	}
	_ = pc.SetMulticastLoopback(true)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(2 * time.Millisecond) // ~500 packets/sec
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_, _ = sendConn.WriteTo(payload, group)
			}
		}
	}()
	defer func() { close(stop); wg.Wait() }()

	// A configured device that is never found is what keeps discovery running: cycles are
	// skipped while every configured device is being read, and skipped entirely when none is
	// configured, so without this the leak path under test would never be entered.
	cfg := &config.Config{
		Interface:     iface,
		FetchInterval: time.Second,
		Devices:       []config.DeviceConfig{{Serial: 1234567890}},
	}
	l := NewListener(cfg, collector.NewCollector())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runtime.GC()
	base := runtime.NumGoroutine()

	go l.Run(ctx)

	sample := func(after time.Duration) int {
		time.Sleep(after)
		runtime.GC()
		return runtime.NumGoroutine()
	}

	// early: the single discovery window (400ms) has closed; whatever piled up is now draining
	// and no new discovery goroutines are being spawned.
	early := sample(1500 * time.Millisecond)
	// late: much later. With bounded discovery the count must not have grown (it only drains);
	// with continuous discovery it keeps climbing with every received packet.
	late := sample(8 * time.Second)

	recv := atomic.LoadInt64(&logger.recv)
	t.Logf("goroutines base=%d early=%d late=%d (sunny recv=%d)", base, early, late, recv)
	if recv == 0 {
		t.Skip("listener connection received no multicast packets; discovery path not exercised")
	}

	// The invariant: once a discovery window closes, no new discovery goroutines are spawned, so
	// the count can only stay flat or drain down. Continuous discovery keeps spawning one per
	// received packet, so late would be far above early.
	assert.LessOrEqual(t, late, early+15,
		"goroutine count kept growing after the discovery window closed (continuous-discovery leak)")
}
