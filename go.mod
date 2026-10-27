module github.com/harishtpj/ais

go 1.27.1

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/joho/godotenv v1.5.1
	github.com/redis/go-redis/v9 v9.23.0
	github.com/rs/zerolog v1.35.1
	github.com/spf13/cobra v1.10.2
	github.com/zhulik/gruby v0.0.0-20240916201602-f69614bfb1b0
	golang.org/x/term v0.46.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cornelk/hashmap v1.0.8 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/zhulik/gruby => ./vendors/gruby
