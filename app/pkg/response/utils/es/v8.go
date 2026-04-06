package es

import (
	"crypto/tls"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/utils/su_logger"
)

type V8 struct {
	*elasticsearch.Client
}

type Config struct {
	CloudId   string
	Key       string
	Addresses []string
	Username  string
	Password  string
}

func New(cnf Config) (es *V8, err error) {
	conf := elasticsearch.Config{
		Addresses:               cnf.Addresses,
		Username:                cnf.Username,
		Password:                cnf.Password,
		CloudID:                 cnf.CloudId,
		APIKey:                  cnf.Key,
		ServiceToken:            "",
		CertificateFingerprint:  "",
		Header:                  nil,
		CACert:                  nil,
		RetryOnStatus:           nil,
		DisableRetry:            false,
		MaxRetries:              0,
		RetryOnError:            nil,
		CompressRequestBody:     false,
		DiscoverNodesOnStart:    false,
		DiscoverNodesInterval:   0,
		EnableMetrics:           false,
		EnableDebugLogger:       false,
		EnableCompatibilityMode: false,
		DisableMetaHeader:       false,
		RetryBackoff:            nil,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Logger:             nil,
		Selector:           nil,
		ConnectionPoolFunc: nil,
	}
	client, err := elasticsearch.NewClient(conf)
	if err != nil {
		su_logger.Error(nil, err, "Error creating the client: %s")
		return
	}
	es = &V8{}
	es.Client = client

	//res, err := es.cli.Info()
	//if err != nil {
	//	log.Fatalf("Error getting response: %s", err)
	//}
	//
	//defer res.Body.Close()
	//log.Println(res)

	return es, nil
}
