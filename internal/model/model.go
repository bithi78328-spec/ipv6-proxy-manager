package model

import "time"

const StateVersion = 1

type State struct {
	Version        int       `json:"version"`
	PublicIPv4     string    `json:"public_ipv4"`
	Interface      string    `json:"interface"`
	IPv6Prefix     string    `json:"ipv6_prefix"`
	NextPort       int       `json:"next_port"`
	NextIPv6Offset uint64    `json:"next_ipv6_offset"`
	AccessToken    string    `json:"access_token"`
	Proxies        []Proxy   `json:"proxies"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Proxy struct {
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	IPv6        string    `json:"ipv6"`
	Username    string    `json:"username"`
	Password    string    `json:"password"`
	Enabled     bool      `json:"enabled"`
	Status      string    `json:"status"`
	LastError   string    `json:"last_error,omitempty"`
	LastChecked time.Time `json:"last_checked,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewState() State {
	return State{
		Version:        StateVersion,
		NextPort:       10000,
		NextIPv6Offset: 1,
		Proxies:        []Proxy{},
	}
}

type Summary struct {
	Total    int `json:"total"`
	Live     int `json:"live"`
	Disabled int `json:"disabled"`
	Failed   int `json:"failed"`
	Checking int `json:"checking"`
}

func Summarize(proxies []Proxy) Summary {
	var out Summary
	out.Total = len(proxies)
	for _, p := range proxies {
		if !p.Enabled || p.Status == "disabled" {
			out.Disabled++
			continue
		}
		switch p.Status {
		case "live":
			out.Live++
		case "checking", "pending":
			out.Checking++
		default:
			out.Failed++
		}
	}
	return out
}
