package connectionpool

type Config struct {
	Backends    []string
	NumConns    int
	EnableCache bool
}
