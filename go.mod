module github.com/runcoil/runcoil

go 1.27

require (
	connectrpc.com/connect/v2 v2.0.2
	connectrpc.com/validate v0.9.0
	github.com/jackc/pgx/v5 v5.12.0
	github.com/lmittmann/tint v1.2.1
	github.com/pressly/goose/v3 v3.29.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/runcoil/runcoil/proto v0.1.0
	github.com/spf13/pflag v1.1.0
	github.com/stretchr/testify v1.12.1
	go.yaml.in/yaml/v3 v3.0.6
	golang.org/x/crypto v0.59.0
	google.golang.org/protobuf v1.36.12
)

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.12-20260825204119-511051f7f437.2 // indirect
	buf.build/go/protovalidate v1.4.0 // indirect
	cel.dev/cel-go v0.32.0 // indirect
	cel.dev/expr v0.25.3 // indirect
	connectrpc.com/connect v1.21.0 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/sethvargo/go-retry v0.5.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20261011100332-faecfb24eb49 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20261005182115-fad411399dd8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261005182115-fad411399dd8 // indirect
)

replace github.com/runcoil/runcoil/proto => ./proto
