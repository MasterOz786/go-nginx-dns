package handlers

import (
	"github.com/gin-gonic/gin"
	"net"
	"strings"
)

type DNSRecord struct {
	Type  string   `json:"type"`
	Name  string   `json:"name"`
	Value []string `json:"value"`
}

type DNSResponse struct {
	Status  string      `json:"status"`
	Domain  string      `json:"domain"`
	Records []DNSRecord `json:"records,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func GetDNSInfo(c *gin.Context) {
	domain := c.Query("domain")
	if domain == "" {
		domain = "google.com"
	}

	response := DNSResponse{
		Status:  "success",
		Domain:  domain,
		Records: []DNSRecord{},
	}

	// Get A records (IPv4)
	ips, err := net.LookupIP(domain)
	if err != nil {
		response.Status = "error"
		response.Error = err.Error()
		c.JSON(500, response)
		return
	}

	var ipv4s, ipv6s []string
	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			ipv4s = append(ipv4s, ipv4.String())
		} else {
			ipv6s = append(ipv6s, ip.String())
		}
	}

	if len(ipv4s) > 0 {
		response.Records = append(response.Records, DNSRecord{
			Type:  "A",
			Name:  domain,
			Value: ipv4s,
		})
	}

	if len(ipv6s) > 0 {
		response.Records = append(response.Records, DNSRecord{
			Type:  "AAAA",
			Name:  domain,
			Value: ipv6s,
		})
	}

	// Get CNAME
	cname, err := net.LookupCNAME(domain)
	if err == nil && cname != domain && cname != domain+"." {
		response.Records = append(response.Records, DNSRecord{
			Type:  "CNAME",
			Name:  domain,
			Value: []string{strings.TrimSuffix(cname, ".")},
		})
	}

	// Get MX records
	mxRecords, err := net.LookupMX(domain)
	if err == nil && len(mxRecords) > 0 {
		var mxValues []string
		for _, mx := range mxRecords {
			mxValues = append(mxValues, strings.TrimSuffix(mx.Host, "."))
		}
		response.Records = append(response.Records, DNSRecord{
			Type:  "MX",
			Name:  domain,
			Value: mxValues,
		})
	}

	// Get TXT records
	txtRecords, err := net.LookupTXT(domain)
	if err == nil && len(txtRecords) > 0 {
		response.Records = append(response.Records, DNSRecord{
			Type:  "TXT",
			Name:  domain,
			Value: txtRecords,
		})
	}

	// Get NS records
	nsRecords, err := net.LookupNS(domain)
	if err == nil && len(nsRecords) > 0 {
		var nsValues []string
		for _, ns := range nsRecords {
			nsValues = append(nsValues, strings.TrimSuffix(ns.Host, "."))
		}
		response.Records = append(response.Records, DNSRecord{
			Type:  "NS",
			Name:  domain,
			Value: nsValues,
		})
	}

	c.JSON(200, response)
}
