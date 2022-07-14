package pool

type Config struct {
	Backends    []string
	NumConns    int
	EnableCache bool
	MaxRetries  int
}
