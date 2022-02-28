package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/go-sockaddr/template"
	"github.com/rs/zerolog"
	flag "github.com/spf13/pflag"
	"github.com/tidwall/finn"
)

var (
	debug   bool
	version bool
	nocolor bool
	trace   bool

	maxDatafileSize int

	bind          string
	dir           string
	logdir        string
	join          string
	consistency   string
	durability    string
	parseSnapshot string

	log zerolog.Logger
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.BoolVarP(&version, "version", "V", false, "display version information")
	flag.BoolVarP(&debug, "debug", "D", false, "enable debug logging")
	flag.BoolVarP(&trace, "trace", "T", false, "enable trace logging")
	// TODO: stop underlying logger from using colors from.. kvnode? when this is called.
	flag.BoolVar(&nocolor, "no-color", false, "do not use colors during logging")

	flag.IntVar(&maxDatafileSize, "max-datafile-size", 1<<20, "maximum datafile size in bytes")

	flag.StringVarP(&bind, "bind", "b", ":4920", "bind/discoverable ip:port")
	flag.StringVarP(&dir, "data", "d", "data", "data directory")
	flag.StringVarP(&logdir, "log-dir", "l", "", "log directory. If blank it will equals --data")
	flag.StringVarP(&join, "join", "j", "", "Join a cluster by providing an address")
	flag.StringVar(&consistency, "consistency", "high", "Consistency (low,medium,high)")
	flag.StringVar(&durability, "durability", "high", "Durability (low,medium,high)")
	flag.StringVar(&parseSnapshot, "parse-snapshot", "", "Parse and output a snapshot to Redis format")

	log = zerolog.New(&zerolog.ConsoleWriter{NoColor: nocolor, Out: os.Stdout}).With().Timestamp().Logger()
}

func main() {
	flag.Parse()

	switch {
	case trace:
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case debug:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	if version {
		fmt.Printf("bitraft version %s", FullVersion())
		os.Exit(0)
	}

	if parseSnapshot != "" {
		err := WriteRedisCommandsFromSnapshot(os.Stdout, parseSnapshot)
		if err != nil {
			log.Warn().Err(err).Msg("failed to parse snapshot")
			os.Exit(1)
		}
		return
	}

	var lconsistency finn.Level
	switch strings.ToLower(consistency) {
	default:
		log.Warn().Msg("invalid --consistency")
	case "low":
		lconsistency = finn.Low
	case "medium", "med":
		lconsistency = finn.Medium
	case "high":
		lconsistency = finn.High
	}

	var ldurability finn.Level
	switch strings.ToLower(durability) {
	default:
		log.Warn().Msg("invalid --durability")
	case "low":
		ldurability = finn.Low
	case "medium", "med":
		ldurability = finn.Medium
	case "high":
		ldurability = finn.High
	}

	if logdir == "" {
		logdir = dir
	}

	mustParse := func(addr string) string {
		r, err := template.Parse(addr)
		if err != nil {
			log.Fatal().Err(err).Msgf("error parsing addr %s: %s", addr, err)
		}
		return r
	}

	log.Debug().Str("bind", bind).Msg("bind raw")
	bind = mustParse(bind)
	log.Debug().Str("bind", bind).Msg("bind parsed")

	if err := ListenAndServe(bind, join, dir, logdir, lconsistency, ldurability); err != nil {
		log.Warn().Msgf("%v", err)
	}
}
