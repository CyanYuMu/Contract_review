package google

import (
	"cloud.google.com/go/bigquery"
	"context"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/encrypt"
	"google.golang.org/api/option"
)

type BigQuery struct {
	*bigquery.Client
	// 平台参数
	ProjectId string
	Region    string
}

type BiqQueryConfig struct {
	CredentialsJson string
	Region          string
	// 平台参数
	ProjectId string
}

func NewBigQuery(ctx context.Context, cnf BiqQueryConfig, opts ...option.ClientOption) (cli *BigQuery, err error) {
	cli = &BigQuery{}
	byteData, err := encrypt.Base64Decode([]byte(cnf.CredentialsJson))
	if err != nil {
		return nil, err
	}
	opts = append(opts, option.WithCredentialsJSON(byteData))
	cli.Client, err = bigquery.NewClient(ctx, cnf.ProjectId, opts...)
	if err != nil {
		return
	}
	cli.ProjectId = cnf.ProjectId
	cli.Region = cnf.Region

	return cli, nil
}
