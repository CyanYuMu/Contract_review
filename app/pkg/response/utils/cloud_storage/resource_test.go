package cloud_storage

import (
	"context"
	"fmt"
	"os"
	"testing"
)

var secretKey = "26a8b5cd97e07dd8bd78e5c3b28d03346b99b5df2567b0d83c9fa17d288a749e"

func TestGetConfig(t *testing.T) {
	sdk := NewResourceSdk(AsiaNortheast3Region, "seaart-api", FileType, secretKey)
	sdk.Dev()

	// Call GetConfig
	config, err := sdk.getConfig(context.Background())
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
		return
	}
	t.Logf("CDN Host: %s", config.CdnHost)
}

func TestUploadObject(t *testing.T) {
	sdk := NewResourceSdk(AsiaNortheast3Region, "test-service", FileType, secretKey)
	//sdk.Dev()

	err := sdk.UploadObject(context.Background(), "temp/test.txt", []byte("test"))
	if err != nil {
		t.Fatalf("UploadObject failed: %v", err)
		return
	}
	t.Logf("Object uploaded successfully")
}

// TestUploadObjects tests the batch upload functionality of the ResourceSdk.
func TestUploadObjects(t *testing.T) {
	sdk := NewResourceSdk(UsCentral1Region, "test-service", FileType, secretKey)
	sdk.Dev()

	objects := []UploadObject{
		{
			ObjectName: "temp/object1.txt",
			File:       []byte("Content of object 1"),
			Attr: &ObjectAttr{
				ContentType: "text/plain",
			},
		},
		{
			ObjectName: "temp/object2.jpg",
			File:       []byte{0xFF, 0xD8, 0xFF, 0xE0}, // JPEG file signature bytes
			Attr: &ObjectAttr{
				ContentType: "image/jpeg",
			},
		},
		{
			ObjectName: "temp/object3.pdf",
			File:       []byte("%PDF-1.4\n%����\n"), // PDF file header bytes
			Attr: &ObjectAttr{
				ContentType: "application/pdf",
			},
		},
	}

	// Call UploadObjects
	results := sdk.UploadObjects(context.Background(), objects)

	for _, result := range results {
		if result.Error != nil {
			t.Fatalf("UploadObjects failed: %v", result.Error)
			return
		}
		t.Logf("Object %s uploaded successfully", result.ObjectName)
	}
}

func TestUploadPreSign(t *testing.T) {
	sdk := NewResourceSdk(UsCentral1Region, "test-service", FileType, secretKey)
	sdk.Dev()

	sig, err := sdk.UploadPreSign(context.Background(), "temp/test.txt", []byte("test"))
	if err != nil {
		t.Fatalf("presign failed: %v", err)
		return
	}
	t.Logf("Object presign successfully %s", sig)
}

func TestReadObject(t *testing.T) {
	sdk := NewResourceSdk(UsCentral1Region, "seaart-api", FileType, secretKey)
	sdk.Dev()

	object, err := sdk.ReadObject(context.Background(), "static/379346f1041ac469a3467ee9679807f7/0/a1207eb16423aa8a2cc8d70cb16da471_high.webp")
	if err != nil {
		t.Fatalf("read object: %v", err)
		return
	}
	t.Logf("read object successfully byte:%d", len(object))

	object, err = sdk.ReadObject(context.Background(), "/static/379346f1041ac469a3467ee9679807f7/0/a1207eb16423aa8a2cc8d70cb16da471_high.webp")
	if err != nil {
		t.Fatalf("read object: %v", err)
		return
	}
	t.Logf("read object successfully byte:%d", len(object))
}

func TestGetObjectAttrs(t *testing.T) {
	sdk := NewResourceSdk(UsCentral1Region, "test-service", FileType, secretKey)
	sdk.Local()

	attr, err := sdk.GetObjectAttrs(context.Background(), "temp/20241011/398d8714-0b5e-422f-b73a-767b63ab261c_/1.png")
	if err != nil {
		panic(err)
	}
	fmt.Println(attr)
}

func TestDelete(t *testing.T) {
	sdk := NewResourceSdk(UsCentral1Region, "test-service", FileType, "4fc79035b3eb4f3a7d40c6968c5e63bde1ad53ec014d03bfcd7c0be55dc27de1")
	sdk.Dev()

	err := sdk.DeleteObject(context.Background(), "2024-10-12/dev-cs559elgfliv7qpsb9ng/ac8f0f3eb93c726a23179fb9c0d5ca358c715017_high.webp")
	if err != nil {
		panic(err)
	}
}

func TestUploadObjectFromFile(t *testing.T) {
	gm := NewGcpManager(nil, GcpManagerParam{
		Region:       UsCentral1Region,
		ServiceName:  "test-service",
		ResourceType: FileType,
		SecretKey:    "4fc79035b3eb4f3a7d40c6968c5e63bde1ad53ec014d03bfcd7c0be55dc27de1",
	})
	f, _ := os.Open("_low.webp")
	gm.UploadObjectFromFile(context.Background(), "temp/low.webp", f)
}

func TestMove(t *testing.T) {
	sdk := NewResourceSdk(UsCentral1Region, "test-service", FileType, "4fc79035b3eb4f3a7d40c6968c5e63bde1ad53ec014d03bfcd7c0be55dc27de1")
	sdk.Local()

	sdk.MoveObject(context.Background(), "seaart-image-dual1", "upload/video/2024-10-11/cs4dpalgfliq7d996h6g.mp4", "seaart-image-us-central1", "temp/upload/video/2024-10-11/cs4dpalgfliq7d996h6g.mp4")
}

func TestOldToNew(t *testing.T) {
	sdk := NewResourceSdk(UsCentral1Region, "test-service", FileType, "4fc79035b3eb4f3a7d40c6968c5e63bde1ad53ec014d03bfcd7c0be55dc27de1")
	//sdk.Local()

	sdk.MoveObject(context.Background(), "seaart-image-us-central1", "temp/temp/upload/video/2024-10-11/cs4dpalgfliq7d996h6g.mp4", "seaart-image-us-central1", "temp/upload/video/2024-10-11/cs4dpalgfliq7d996h6g.mp4")
}
