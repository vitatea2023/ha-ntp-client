module github.com/vitatea2023/ha-ntp-client

go 1.23.0

require (
	github.com/facebook/time v0.0.0-20220412213853-172b08dfa7db
	github.com/spf13/cobra v1.8.0
	github.com/vitatea2023/ha-dns-client v0.0.0-00010101000000-000000000000
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/vitatea2023/ha-dns-client => ../ha-dns-client

replace github.com/facebook/time => ../time

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/miekg/dns v1.1.66 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	golang.org/x/mod v0.24.0 // indirect
	golang.org/x/net v0.39.0 // indirect
	golang.org/x/sync v0.13.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/tools v0.32.0 // indirect
)
