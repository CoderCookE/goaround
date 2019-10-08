package connectionpool

import (
	"bufio"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

type pool struct {
	connections     chan *connection
	healthChecks    map[string]*healthChecker
	client          *http.Client
	connsPerBackend int
}

//Exported method for creation of a connection-pool takes []string
//ex: ['http://localhost:9000','http://localhost:9000']
func New(backends []string, connsPerBackend int) *pool {
	var maxRequests int
	backendCount := int(math.Max(float64(len(backends)), float64(1)))

	if backendCount > 0 {
		maxRequests = connsPerBackend * backendCount
	}

	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		MaxIdleConns:          maxRequests + backendCount,
		IdleConnTimeout:       120 * time.Second,
		MaxConnsPerHost:       connsPerBackend + 1,
		MaxIdleConnsPerHost:   connsPerBackend + 1,
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: tr,
	}

	connectionPool := &pool{
		connections:     make(chan *connection, maxRequests),
		healthChecks:    make(map[string]*healthChecker),
		client:          client,
		connsPerBackend: connsPerBackend,
	}

	poolConnections := make([]*connection, 0)

	for _, backend := range backends {
		newConnection := connectionPool.addBackend(backend)
		poolConnections = append(poolConnections, newConnection)
	}

	shuffle(poolConnections, connectionPool.connections)
	go connectionPool.ListenForBackendChanges()

	return connectionPool
}

func shuffle(conns []*connection, ch chan *connection) {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(conns), func(i, j int) {
		conns[i], conns[j] = conns[j], conns[i]
	})

	for _, conn := range conns {
		ch <- conn
	}
}

//Exported method for passing a request to a connection from the pool
//Returns a 503 status code if request is unsuccessful
func (p *pool) Fetch(w http.ResponseWriter, r *http.Request) {
	select {
	case connection := <-p.connections:
		defer func() {
			p.connections <- connection
		}()

		err := connection.get(w, r)
		if err != nil {
			log.Printf("retrying err with request: %s", err.Error())
			p.Fetch(w, r)
		}
	default:
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func (p *pool) Shutdown() {
	for _, hc := range p.healthChecks {
		hc.Shutdown()
	}
}

func (p *pool) ListenForBackendChanges() {
	const SockAddr = "/tmp/goaround.sock"

	if err := os.RemoveAll(SockAddr); err != nil {
		log.Fatal(err)
	}

	l, err := net.Listen("unix", SockAddr)
	if err != nil {
		log.Fatal("listen error:", err)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal("accept error:", err)
		}

		scanner := bufio.NewScanner(conn)
		scanner.Scan()
		updated := strings.Split(scanner.Text(), ",")

		var currentBackends []string
		for k := range p.healthChecks {
			currentBackends = append(currentBackends, k)
		}

		added, removed := difference(currentBackends, updated)

		for _, removedBackend := range removed {
			if len(added) > 0 {
				var new string
				new, added = added[0], added[1:]
				p.healthChecks[removedBackend].Reuse(new)

			} else {
				p.healthChecks[removedBackend].Remove()
			}

			delete(p.healthChecks, removedBackend)
		}

		for _, addedBackend := range added {
			println(addedBackend)
		}
	}
}

func difference(original []string, updated []string) (added []string, removed []string) {
	oldBackends := make(map[string]bool)
	for _, i := range original {
		oldBackends[i] = true
	}

	newBackends := make(map[string]bool)
	for _, i := range updated {
		newBackends[i] = true
	}

	for _, i := range updated {
		if _, ok := oldBackends[i]; !ok {
			added = append(added, i)
		}
	}

	for _, i := range original {
		if _, ok := newBackends[i]; !ok {
			removed = append(removed, i)
		}
	}

	return
}

func (p *pool) addBackend(backend string) (configuredConn *connection) {
	url, err := url.ParseRequestURI(backend)
	if err != nil {
		log.Printf("error parsing backend url: %s", backend)
	} else {
		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.Transport = p.client.Transport

		configuredConn, err = newConnection(proxy, backend)
		if err != nil {
			log.Printf("Error adding connection for: %s", backend)
		} else {
			backendConnections := make([]chan message, p.connsPerBackend)

			for i := 0; i < p.connsPerBackend; i++ {
				backendConnections[i] = configuredConn.messages
			}

			hc := &healthChecker{
				client:      p.client,
				subscribers: backendConnections,
				backend:     configuredConn.backend,
				done:        make(chan bool, 1),
			}

			p.healthChecks[backend] = hc
			go hc.Start()
		}
	}

	return configuredConn
}
