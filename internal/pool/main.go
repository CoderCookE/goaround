package pool

import (
	"bufio"
	"bytes"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"

	"github.com/CoderCookE/goaround/internal/connection"
	"github.com/CoderCookE/goaround/internal/healthcheck"
	"github.com/CoderCookE/goaround/internal/stats"
)

type pool struct {
	sync.RWMutex
	connections     chan *connection.Connection
	healthChecks    map[string]*healthcheck.HealthChecker
	client          *http.Client
	connsPerBackend int
	cacheEnabled    bool
	cache           *ristretto.Cache
}

//Exported method for creation of a connection-pool takes []string
//ex: ['http://localhost:9000','http://localhost:9000']
func New(c *Config) *pool {
	backends := c.Backends
	connsPerBackend := c.NumConns
	cacheEnabled := c.EnableCache

	var maxRequests int
	backendCount := int(math.Max(float64(len(backends)), float64(1)))

	if backendCount > 0 {
		maxRequests = connsPerBackend * backendCount * 2
	}

	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 10 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}

	client := &http.Client{
		Timeout:   60 * time.Second,
		Transport: tr,
	}

	var cache *ristretto.Cache
	var err error
	if cache, err = buildCache(cacheEnabled); err != nil {
		log.Printf("Error creating cache: %v", err)
		cacheEnabled = false
	}

	connectionPool := &pool{
		connections:     make(chan *connection.Connection, maxRequests),
		healthChecks:    make(map[string]*healthcheck.HealthChecker),
		client:          client,
		connsPerBackend: connsPerBackend,
		cache:           cache,
		cacheEnabled:    cacheEnabled,
	}

	poolConnections := []*connection.Connection{}

	startup := &sync.WaitGroup{}
	for _, backend := range backends {
		startup.Add(1)
		poolConnections = connectionPool.addBackend(poolConnections, backend, startup)
	}

	shuffle(poolConnections, connectionPool.connections)

	go connectionPool.ListenForBackendChanges(startup)

	return connectionPool
}

func shuffle(conns []*connection.Connection, ch chan *connection.Connection) {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(conns), func(i, j int) {
		conns[i], conns[j] = conns[j], conns[i]
	})

	for _, conn := range conns {
		stats.AvailableConnectionsGauge.WithLabelValues("available").Add(1)
		ch <- conn
	}
}

func buildCache(cacheEnabled bool) (*ristretto.Cache, error) {
	if !cacheEnabled {
		return nil, nil
	}

	cache, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})

	return cache, err
}

//Exported method for passing a request to a connection from the pool
//Returns a 503 status code if request is unsuccessful
func (p *pool) Fetch(w http.ResponseWriter, r *http.Request) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	start := time.Now()

	var conn *connection.Connection
	for {
		select {
		case conn = <-p.connections:
			duration := time.Since(start).Seconds()
			stats.Durations.WithLabelValues("get_connection").Observe(duration)
			stats.AvailableConnectionsGauge.WithLabelValues("in_use").Add(1)

			defer func() {
				stats.AvailableConnectionsGauge.WithLabelValues("in_use").Sub(1)
				if !conn.Shut {
					p.connections <- conn
				}
			}()

			if p.cache != nil && r.Method == "GET" {
				value, found := p.cache.Get(r.URL.Path)
				if found {
					stats.CacheCounter.WithLabelValues("hit").Add(1)
					res := value.(string)
					_, err := w.Write([]byte(res))
					if err != nil {
						log.Printf("Error writing: %s", err.Error())
					}
					return
				}

				stats.CacheCounter.WithLabelValues("miss").Add(1)
			}

			usableProxy, err := conn.Get()
			if err != nil {
				log.Printf("retrying err with request: %s", err.Error())
				stats.RequestCounter.WithLabelValues("retry").Add(1)
				p.Fetch(w, r)
			} else {
				usableProxy.ServeHTTP(w, r)
			}
			return
		case <-ticker.C:
			stats.RequestCounter.WithLabelValues("no_connection").Add(1)
			w.WriteHeader(http.StatusGatewayTimeout)
			w.Write([]byte("Gateway Timeout"))
			return
		}
	}
}

func (p *pool) Shutdown() {
	p.RLock()
	for _, hc := range p.healthChecks {
		hc.Shutdown()
	}
	p.RUnlock()
}

func (p *pool) ListenForBackendChanges(startup *sync.WaitGroup) {
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
		for scanner.Scan() {
			updated := strings.Split(scanner.Text(), ",")
			p.Lock()
			var currentBackends []string
			for k := range p.healthChecks {
				currentBackends = append(currentBackends, k)
			}

			added, removed := difference(currentBackends, updated)
			log.Printf("Adding: %s", added)
			log.Printf("Removing: %s", removed)

			for _, removedBackend := range removed {
				if len(added) > 0 {
					var new string
					new, added = added[0], added[1:]
					endpoint, err := url.ParseRequestURI(new)
					if err != nil {
						log.Printf("Error adding backend, %s", new)
					} else {
						proxy := httputil.NewSingleHostReverseProxy(endpoint)
						proxy.Transport = p.client.Transport
						proxy.ErrorHandler = errorHandler
						p.setupCache(proxy)

						newHC := p.healthChecks[removedBackend].Reuse(new, proxy)
						p.healthChecks[new] = newHC
					}
				} else {
					p.healthChecks[removedBackend].Shutdown()
				}

				delete(p.healthChecks, removedBackend)
			}

			poolConnections := []*connection.Connection{}

			wg := &sync.WaitGroup{}
			for _, addedBackend := range added {
				wg.Add(1)
				poolConnections = p.addBackend(poolConnections, addedBackend, wg)
			}

			shuffle(poolConnections, p.connections)
			p.Unlock()
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

func (p *pool) addBackend(connections []*connection.Connection, backend string, startup *sync.WaitGroup) []*connection.Connection {
	endpoint, err := url.ParseRequestURI(backend)
	if err != nil {
		log.Printf("error parsing backend url: %s", backend)
	} else {
		proxy := httputil.NewSingleHostReverseProxy(endpoint)
		proxy.ErrorHandler = errorHandler
		proxy.Transport = p.client.Transport
		p.setupCache(proxy)

		backendConnections := make([]chan connection.Message, p.connsPerBackend)
		for i := 0; i < p.connsPerBackend; i++ {
			startup.Add(1)
			configuredConn := connection.NewConnection(proxy, backend, startup)
			connections = append(connections, configuredConn)
			backendConnections[i] = configuredConn.Messages
		}

		hc := healthcheck.New(
			p.client,
			backendConnections,
			backend,
			false,
		)

		p.healthChecks[backend] = hc

		go hc.Start(startup)
	}

	startup.Wait()
	return connections
}

func errorHandler(w http.ResponseWriter, r *http.Request, e error) {
	stats.RequestCounter.WithLabelValues("http_error").Add(1)
	return
}

func (p *pool) setupCache(proxy *httputil.ReverseProxy) {
	if p.cache != nil {
		cacheResponse := func(r *http.Response) error {
			body, err := ioutil.ReadAll(r.Body)
			cacheable := string(body)
			r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

			path := r.Request.URL.Path
			if err == nil {
				p.cache.Set(path, cacheable, 1)
			}

			return nil
		}

		proxy.ModifyResponse = cacheResponse
	}
}
