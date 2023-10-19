module github.com/ServiceWeaver/weaver

go 1.21

require (
	github.com/BurntSushi/toml v1.3.2
	github.com/DataDog/hyperloglog v0.0.0-20220804205443-1806d9b66146
	github.com/alecthomas/chroma/v2 v2.2.0
	github.com/fsnotify/fsnotify v1.6.0
	github.com/go-sql-driver/mysql v1.7.1
	github.com/goburrow/cache v0.1.4
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/google/cel-go v0.17.1
	github.com/google/go-cmp v0.5.9
	github.com/google/pprof v0.0.0-20230705174524-200ffdc848b8
	github.com/google/uuid v1.3.1
	github.com/hashicorp/golang-lru/v2 v2.0.1
	github.com/lightstep/varopt v1.3.0
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/testcontainers/testcontainers-go v0.24.1
	github.com/testcontainers/testcontainers-go/modules/mysql v0.24.1
	github.com/yuin/goldmark v1.4.15
	github.com/yuin/goldmark-highlighting/v2 v2.0.0-20220924101305-151362477c87
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.44.0
	go.opentelemetry.io/otel v1.18.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.16.0
	go.opentelemetry.io/otel/sdk v1.16.0
	go.opentelemetry.io/otel/trace v1.18.0
	golang.org/x/crypto v0.14.0
	golang.org/x/exp v0.0.0-20230713183714-613f0c0eb8a1
	golang.org/x/image v0.5.0
	golang.org/x/sync v0.4.0
	golang.org/x/term v0.13.0
	golang.org/x/text v0.13.0
	golang.org/x/tools v0.14.0
	google.golang.org/genproto/googleapis/api v0.0.0-20230717213848-3f92550aa753
	google.golang.org/protobuf v1.31.0
	gorm.io/driver/postgres v1.5.3
	gorm.io/gorm v1.25.5
	modernc.org/sqlite v1.24.0
)

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/DataDog/mmh3 v0.0.0-20210722141835-012dc69a9e49 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/Microsoft/hcsshim v0.11.0 // indirect
	github.com/antlr/antlr4/runtime/Go/antlr/v4 v4.0.0-20230305170008-8188dc5388df // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/containerd/containerd v1.7.6 // indirect
	github.com/cpuguy83/dockercfg v0.3.1 // indirect
	github.com/dlclark/regexp2 v1.7.0 // indirect
	github.com/docker/distribution v2.8.2+incompatible // indirect
	github.com/docker/docker v24.0.6+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/dustin/randbo v0.0.0-20140428231429-7f1b564ca724 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgx/v5 v5.4.3 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/compress v1.16.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.7 // indirect
	github.com/mattn/go-isatty v0.0.16 // indirect
	github.com/moby/patternmatcher v0.5.0 // indirect
	github.com/moby/sys/sequential v0.5.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0-rc4 // indirect
	github.com/opencontainers/runc v1.1.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/shirou/gopsutil/v3 v3.23.7 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/stoewer/go-strcase v1.2.0 // indirect
	github.com/tklauser/go-sysconf v0.3.11 // indirect
	github.com/tklauser/numcpus v0.6.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	go.opentelemetry.io/otel/metric v1.18.0 // indirect
	golang.org/x/mod v0.13.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230706204954-ccb25ca9f130 // indirect
	google.golang.org/grpc v1.57.0 // indirect
	lukechampine.com/uint128 v1.2.0 // indirect
	modernc.org/cc/v3 v3.40.0 // indirect
	modernc.org/ccgo/v3 v3.16.13 // indirect
	modernc.org/libc v1.22.5 // indirect
	modernc.org/mathutil v1.5.0 // indirect
	modernc.org/memory v1.5.0 // indirect
	modernc.org/opt v0.1.3 // indirect
	modernc.org/strutil v1.1.3 // indirect
	modernc.org/token v1.0.1 // indirect
)
