//go:generate go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
//go:generate protoc -I. -I/usr/local/include --go_out=.  --go_opt=paths=source_relative network.proto signer.proto state_machine.proto

package consensus