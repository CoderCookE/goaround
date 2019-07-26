package customflags

import "fmt"

type Backend []string

func (i *Backend) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func (i *Backend) String() string {
	return fmt.Sprintf("%s", *i)
}
