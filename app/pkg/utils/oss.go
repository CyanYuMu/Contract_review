package utils

import (
	"context"
	"contract_review/app/internal/global"
	"fmt"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"mime/multipart"
)

func AliyunOss(ctx context.Context, fileName string, file *multipart.FileHeader) (string, error) {
	config := global.Config.OSS
	client, err := oss.New(config.Endpoint, config.AccessKey, config.SecretKey)
	if err != nil {
		return "", err
	}
	bucket, err := client.Bucket(config.BucketName)
	if err != nil {
		return "", err
	}

	fileData, err := file.Open()
	defer fileData.Close()

	err = bucket.PutObject(fileName, fileData)
	if err != nil {
		return "", err
	}
	imagePath := "https://" + config.BucketName + "." + config.Endpoint + "/" + fileName
	global.Log.Info(fmt.Sprintf("文件上传到: %s", imagePath))
	return imagePath, nil
}
