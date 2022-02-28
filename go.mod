module git.mills.io/prologic/bitraft

go 1.16

replace labix.org/v2/mgo => gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22

replace launchpad.net/gocheck => github.com/go-check/check v0.0.0-20190902080502-41f04d3bba15

require (
	git.tcp.direct/Mirrors/bitcask-mirror v0.0.0-20220228092422-1ec4297c7e34
	github.com/golang/leveldb v0.0.0-20170107010102-259d9253d719 // indirect
	github.com/hashicorp/go-sockaddr v1.0.2
	github.com/onsi/ginkgo v1.8.0 // indirect
	github.com/onsi/gomega v1.5.0 // indirect
	github.com/rs/zerolog v1.26.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/finn v0.1.2
	github.com/tidwall/redcon v1.4.1
	golang.org/x/exp v0.0.0-20201008143054-e3b2a7f2fdc7 // indirect
)
