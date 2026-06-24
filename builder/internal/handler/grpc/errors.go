package grpc

import "fmt"

func errRequired(field string) error {
	return fmt.Errorf("%s is required", field)
}
