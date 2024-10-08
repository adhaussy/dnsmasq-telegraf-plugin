package dnsmasq

import (
	"fmt"
	"strconv"
	"strings"
	"os"

	"github.com/miekg/dns"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
)

type ResultType uint64

const (
	Success ResultType = 0
	Timeout            = 1
	Error              = 2
)

type Dnsmasq struct {
	c *dns.Client

	// Dnsmasq server IP address and port
	Server string
}

var sampleConfig = `
  ## Dnsmasq server IP address and port.
  # server = "127.0.0.1:53"
`

func (d *Dnsmasq) SampleConfig() string {
	return sampleConfig
}

func (d *Dnsmasq) Description() string {
	return "Read Dnsmasq metrics by dns query"
}

func (d *Dnsmasq) Gather(acc telegraf.Accumulator) error {
	d.setDefaultValues()
	fields := make(map[string]interface{}, 2)
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("Failed to get hostname:" , err)
	}
	tags := map[string]string{
		"server": d.Server,
		"host": strings.Split(hostname, ".")[0],
	}
	metrics := []string{
		"cachesize.bind.",
		"insertions.bind.",
		"evictions.bind.",
		"misses.bind.",
		"hits.bind.",
		"auth.bind.",
		"servers.bind.",
	}
	for _, metricName := range metrics {
		msg := &dns.Msg{
			MsgHdr: dns.MsgHdr{
				Id:               dns.Id(),
				RecursionDesired: true,
			},
			Question: []dns.Question{
				question(metricName),
			},
		}
		in, _, err := d.c.Exchange(msg, d.Server)
		if err != nil {
			return err
		}
		err = processResponse(in, metricName, fields)
		if err != nil {
			return err
		}
	}

	acc.AddFields("dnsmasq", fields, tags)
	return nil
}

func processResponse(response *dns.Msg, metricName string, fields map[string]interface{}) error {
	for _, a := range response.Answer {
		txt, ok := a.(*dns.TXT)
		if !ok {
			continue
		}
		switch txt.Hdr.Name {
		case "servers.bind.":
			for _, str := range txt.Txt {
				arr := strings.Fields(str)
				if got, want := len(arr), 3; got != want {
					return fmt.Errorf("stats DNS record servers.bind.: unexpeced number of argument in record: got %d, want %d", got, want)
				}
				queries, err := strconv.ParseFloat(arr[1], 64)
				if err != nil {
					return err
				}
				failedQueries, err := strconv.ParseFloat(arr[2], 64)
				if err != nil {
					return err
				}
				fields["queries"] = queries
				fields["queries_failed"] = failedQueries
			}
		default:
			if got, want := len(txt.Txt), 1; got != want {
				return fmt.Errorf("stats DNS record %q: unexpected number of replies: got %d, want %d", txt.Hdr.Name, got, want)
			}
			f, err := strconv.ParseFloat(txt.Txt[0], 64)
			if err != nil {
				return err
			}
			names := strings.Split(txt.Hdr.Name, ".")
			if len(names) > 0 {
				fields[names[0]] = f
			}
		}
	}
	return nil
}

func (d *Dnsmasq) setDefaultValues() {
	if d.Server == "" {
		d.Server = "127.0.0.1:53"
	}
}

func question(name string) dns.Question {
	return dns.Question{
		Name:   name,
		Qtype:  dns.TypeTXT,
		Qclass: dns.ClassCHAOS,
	}
}

func init() {
	dnsClient := &dns.Client{
		SingleInflight: true,
	}
	inputs.Add("dnsmasq", func() telegraf.Input {
		return &Dnsmasq{c: dnsClient}
	})
}
