package google

import (
	"context"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"gitlab.internal.ops.haiyiai.tech/seaart-web-server/seago/tool/encrypt"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type FirebaseConfig struct {
	// Credentials CredentialsFile 二选一
	// CredentialsFile 优先级高于 CredentialsJson
	// CredentialsJson 需要经过base64编码
	CredentialsJson string
	CredentialsFile string
	ProjectId       string
	AccountId       string
	Bucket          string
	DatabaseId      string
	// 连接池大小
	PoolSize int
	// 是否禁用TLS证书验证（用于解决证书验证问题）
	DisableTLSVerify bool
}

func NewFirebase(ctx context.Context, cnf FirebaseConfig) (app *firebase.App, err error) {
	var opts = make([]option.ClientOption, 0, 3)
	var opt option.ClientOption
	if cnf.CredentialsFile != "" {
		opt = option.WithCredentialsFile(cnf.CredentialsFile)
	} else {
		deData, err := encrypt.Base64Decode([]byte(cnf.CredentialsJson))
		if err != nil {
			return nil, err
		}
		opt = option.WithCredentialsJSON(deData)
	}

	opts = append(opts, opt)

	// 如果禁用TLS验证，添加不安全的传输选项
	if cnf.DisableTLSVerify {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	c := &firebase.Config{
		AuthOverride: nil,
		//DatabaseURL:      cnf.DatabaseURL,
		ProjectID:        cnf.ProjectId,
		ServiceAccountID: cnf.AccountId,
		StorageBucket:    cnf.Bucket,
	}

	if cnf.PoolSize > 0 {
		opts = append(opts, option.WithGRPCConnectionPool(cnf.PoolSize))
	}

	return firebase.NewApp(ctx, c, opts...)
}

func NewFirestoreWithDatabase(ctx context.Context, projectId string, databaseId string, cnf FirebaseConfig) (inst *Firestore, err error) {
	var opts = make([]option.ClientOption, 0, 3)
	var opt option.ClientOption
	if cnf.CredentialsFile != "" {
		opt = option.WithCredentialsFile(cnf.CredentialsFile)
	} else {
		deData, err := encrypt.Base64Decode([]byte(cnf.CredentialsJson))
		if err != nil {
			return nil, err
		}
		opt = option.WithCredentialsJSON(deData)
	}

	opts = append(opts, opt)

	// 如果禁用TLS验证，添加不安全的传输选项
	if cnf.DisableTLSVerify {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	}

	if cnf.PoolSize > 0 {
		opts = append(opts, option.WithGRPCConnectionPool(cnf.PoolSize))
	}
	cli, err := firestore.NewClientWithDatabase(ctx, projectId, databaseId, opts...)
	if err != nil {
		return nil, err
	}

	inst = &Firestore{}
	inst.Client = cli
	return inst, nil
}
