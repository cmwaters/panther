//go:generate go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
//go:generate go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
//go:generate protoc -I ../proto/consensus/v1 -I/usr/local/include --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative consensus.proto application.proto network.proto signer.proto

package consensus
