package pool

import (
	"bufio"
	"bytes"
	"fmt"
	"hash/fnv"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto"

	"github.com/CoderCookE/goaround/internal/connection"
	"github.com/CoderCookE/goaround/internal/healthcheck"
	"github.com/CoderCookE/goaround/internal/stats"
)

type attempts int

const (
	attemptsKey attempts = iota
)

type Pool struct {
	sync.RWMutex
	client          *http.Client
	connsPerBackend int
	hashRing        *ConsistentHash
	healthChecks    map[string]*healthcheck.HealthChecker
	cache           *ristretto.Cache
	maxRetries      int
}

type ConsistentHash struct {
	sync.RWMutex
	keys    []uint32
	hashMap map[uint32]*connection.Connection
}

// NewConsistentHash initializes a new ConsistentHash
func NewConsistentHash() *ConsistentHash {
	return &ConsistentHash{
		hashMap: make(map[uint32]*connection.Connection),
	}
}

// Add adds a new connection to the consistent hash
func (ch *ConsistentHash) Add(conn *connection.Connection) {
	ch.Lock()
	defer ch.Unlock()
	hash := hashURL(conn.Backend)

	ch.keys = append(ch.keys, hash)
	ch.hashMap[hash] = conn
	sort.Slice(ch.keys, func(i, j int) bool {
		return ch.keys[i] < ch.keys[j]
	})
}

// Get retrieves the connection for a given URL using consistent hashing
func (ch *ConsistentHash) Get(url string) *connection.Connection {
	if len(ch.keys) == 0 {
		return nil // No connections available
	}

	hash := hashURL(url)
	ch.RLock()
	defer ch.RUnlock()

	// Find the appropriate node
	idx := sort.Search(len(ch.keys), func(i int) bool {
		return ch.keys[i] >= hash
	})

	if idx == len(ch.keys) {
		// Wrap around to the first node
		idx = 0
	}

	return ch.hashMap[ch.keys[idx]]
}

// New creates a new connection pool from the given configuration.
func New(c *Config) *Pool {
	backends := c.Backends
	connsPerBackend := c.NumConns
	cacheEnabled := c.EnableCache
	maxRetries := c.MaxRetries

	client := createHTTPClient()
	cache, err := buildCache(cacheEnabled)
	if err != nil {
		log.Printf("Error creating cache: %v", err)
	}

	connectionPool := &Pool{
		client:          client,
		connsPerBackend: connsPerBackend,
		cache:           cache,
		maxRetries:      maxRetries,
		hashRing:        NewConsistentHash(),
		healthChecks:    make(map[string]*healthcheck.HealthChecker),
	}

	startup := &sync.WaitGroup{}
	connectionPool.initializeBackends(backends, startup)

	go connectionPool.listenForBackendChanges(startup)

	return connectionPool
}

func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 10 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}
}

func buildCache(cacheEnabled bool) (*ristretto.Cache, error) {
	if !cacheEnabled {
		return nil, nil
	}

	return ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // 10M keys to track frequency.
		MaxCost:     1 << 30, // 1GB maximum cost.
		BufferItems: 64,      // 64 keys per Get buffer.
	})
}

func (cp *Pool) initializeBackends(backends []string, startup *sync.WaitGroup) {
	for _, backend := range backends {
		startup.Add(1)
		cp.addBackend(backend, startup)
	}
}

func hashURL(url string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(url))
	return h.Sum32()
}

func (p *Pool) Fetch(w http.ResponseWriter, r *http.Request) {
	lookupURL := r.URL.Query().Get("url")

	attempt := getAttemptCount(r)
	log.Printf("Attempt: %d", attempt)

	// Extract the URL from query params
	requestURL := r.URL.Query().Get("url")

	if requestURL == "" {
		requestURL = r.URL.String()
	}

	log.Printf("Requesting: %s", requestURL)

	parsedURL, err := url.Parse(requestURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		log.Printf("Invalid %s", requestURL)
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	var backendURL string

	// If the attempt is 0, use the hashing mechanism to select a backend

	var proxy *httputil.ReverseProxy

	if attempt == 0 {
		conn := p.hashRing.Get(requestURL)
		if conn == nil {
			http.Error(w, "No backend found", http.StatusInternalServerError)
			return
		}

		backendURL = conn.Backend
		log.Printf("Selected backend: %s for URL: %s", backendURL, requestURL)

		var err error
		proxy, err = conn.Get() // Assuming Get() returns the proxy
		if err != nil || proxy == nil {
			log.Printf("Error retrieving proxy: %v", err)
			http.Error(w, "Failed to get proxy", http.StatusInternalServerError)
			return
		}
	} else {
		// If attempts > 0, proxy directly to the URL
		backendURL = requestURL
		log.Printf("Retrying final request directly to: %s", backendURL)
		// Parse backend URL
		parsedBackendURL, err := url.Parse(backendURL)
		if err != nil || parsedBackendURL == nil {
			http.Error(w, "Invalid backend URL", http.StatusInternalServerError)
			return
		}

		query := r.URL.Query()
		query.Del("url")
		r.URL.RawQuery = query.Encode()

		proxy = httputil.NewSingleHostReverseProxy(parsedBackendURL)
		proxy.ErrorHandler = p.errorHandler
		proxy.Transport = p.client.Transport
		p.setupCache(proxy)
	}

	print("checking cache: ", lookupURL)

	cachedResponse := p.getCachedResponse(lookupURL)
	if cachedResponse != "" {
		log.Printf("Cache hit for URL: %s", lookupURL)
		w.Write([]byte(cachedResponse))
		return
	}
	// Set attempt count in the header
	r.Header.Set("X-Attempt-Count", fmt.Sprintf("%d", attempt))

	// Serve the request using the proxy
	proxy.ServeHTTP(w, r)
}

func (p *Pool) serveRequest(w http.ResponseWriter, r *http.Request, conn *connection.Connection) error {
	usableProxy, err := conn.Get()
	if err != nil {
		return err
	}

	usableProxy.ServeHTTP(w, r)
	return nil
}

func getAttemptCount(r *http.Request) int {
	if attemptHeader := r.Header.Get("X-Attempt-Count"); attemptHeader != "" {
		var attempt int
		fmt.Sscanf(attemptHeader, "%d", &attempt)
		return attempt + 1
	}

	return 0
}

func (p *Pool) makeRequest(w http.ResponseWriter, backendURL string, originalRequest *http.Request) error {
	// Create a new request with the same method, URL, and body
	req, err := http.NewRequest(originalRequest.Method, backendURL, originalRequest.Body)
	if err != nil {
		return fmt.Errorf("failed to create new request: %w", err)
	}

	// Copy query parameters
	req.URL.RawQuery = originalRequest.URL.RawQuery

	// Copy headers from the original request
	for key, values := range originalRequest.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Initialize an HTTP client with a timeout
	client := &http.Client{Timeout: 10 * time.Second}

	fmt.Printf("Making request to %s\n", req.URL.String())

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Copy the response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Write the response status code
	w.WriteHeader(resp.StatusCode)

	// Copy the response body to the ResponseWriter
	if _, err = io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("failed to copy response body: %w", err)
	}

	return nil
}

func (p *Pool) recordFetchMetrics(start time.Time, conn *connection.Connection, attempt int) {
	duration := time.Since(start).Seconds()
	stats.Durations.WithLabelValues("get_connection").Observe(duration)
	stats.AvailableConnectionsGauge.WithLabelValues("in_use").Add(1)

	defer func() {
		stats.AvailableConnectionsGauge.WithLabelValues("in_use").Sub(1)
		stats.Attempts.WithLabelValues().Observe(float64(attempt))
		duration = time.Since(start).Seconds()
		stats.Durations.WithLabelValues("return_connection").Observe(duration)
	}()
}

func (p *Pool) getCachedResponse(path string) string {
	if value, found := p.cache.Get(path); found {
		log.Printf("Cache Hit: %s", path)
		stats.CacheCounter.WithLabelValues(path, "hit").Add(1)
		return value.(string)
	}

	stats.CacheCounter.WithLabelValues(path, "miss").Add(1)
	log.Printf("Cache Miss: %s", path)
	return ""
}

func (p *Pool) Shutdown() {
	p.RLock()
	defer p.RUnlock()

	for _, hc := range p.healthChecks {
		hc.Shutdown()
	}
}

func (p *Pool) listenForBackendChanges(startup *sync.WaitGroup) {
	const sockAddr = "/tmp/goaround.sock"

	if err := os.RemoveAll(sockAddr); err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("unix", sockAddr)
	if err != nil {
		log.Fatal("Listen error:", err)
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal("Accept error:", err)
		}

		go p.handleBackendUpdates(conn, startup)
	}
}

func (p *Pool) handleBackendUpdates(conn net.Conn, startup *sync.WaitGroup) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		updated := strings.Split(scanner.Text(), ",")
		p.Lock()
		p.updateBackends(updated)
		p.Unlock()
	}
}

func (p *Pool) updateBackends(updated []string) {
	currentBackends := getCurrentBackends(p.healthChecks)

	added, removed := difference(currentBackends, updated)
	log.Printf("Adding: %v", added)
	log.Printf("Removing: %v", removed)

	p.removeBackends(removed)
	p.addBackends(added)
}

func getCurrentBackends(healthChecks map[string]*healthcheck.HealthChecker) []string {
	var current []string
	for k := range healthChecks {
		current = append(current, k)
	}
	return current
}

func (p *Pool) removeBackends(removed []string) {
	for _, backend := range removed {
		if healthChecker, exists := p.healthChecks[backend]; exists {
			healthChecker.Shutdown()
			delete(p.healthChecks, backend)
		}
	}
}

func (p *Pool) addBackends(added []string) {
	for _, backend := range added {
		p.addBackend(backend, nil)
	}
}

func (p *Pool) addBackend(backend string, startup *sync.WaitGroup) {
	endpoint, err := url.ParseRequestURI(backend)
	if err != nil {
		log.Printf("Error parsing backend URL: %s", backend)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(endpoint)
	proxy.ErrorHandler = p.errorHandler
	proxy.Transport = p.client.Transport
	p.setupCache(proxy)

	conn := connection.NewConnection(proxy, backend, startup)
	p.hashRing.Add(conn)
}

func difference(original, updated []string) (added, removed []string) {
	oldBackends := make(map[string]struct{}, len(original))
	for _, i := range original {
		oldBackends[i] = struct{}{}
	}

	newBackends := make(map[string]struct{}, len(updated))
	for _, i := range updated {
		newBackends[i] = struct{}{}
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

func (p *Pool) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	host := fmt.Sprintf("%s:%s", r.URL.Hostname(), r.URL.Port())
	stats.RequestCounter.WithLabelValues(host, "backend_error").Add(1)
	p.Fetch(w, r)
}

func (p *Pool) setupCache(proxy *httputil.ReverseProxy) {
	proxy.ModifyResponse = func(r *http.Response) error {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return err
		}
		r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

		cacheable := string(body) // Convert the body to a string

		if err == nil {
			print("adding: ", r.Request.URL.String())
			p.cache.Set(r.Request.URL.String(), cacheable, 1)
		}

		return nil
	}
}
