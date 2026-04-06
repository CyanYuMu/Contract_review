package es

import (
	"context"
	"fmt"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/elastic/go-elasticsearch/v8/esutil"
	"github.com/tidwall/gjson"
)

/*ScrollScan
* @Description: 半成品
* @param ctx
* @param client
* @param index
* @param body
* @param batchSize
* @return err
 */
func ScrollScan(ctx context.Context, client *elasticsearch.Client, index string, body string, batchSize int) (err error) {
	scrollRequest := esapi.ScrollRequest{
		Body:   esutil.NewJSONReader(body),
		Scroll: time.Minute,
	}

	response, err := scrollRequest.Do(ctx, client)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	gData := gjson.Parse(response.String()[9:])
	for {
		numItems := 0
		gData.Get("hits.hits").ForEach(func(key, value gjson.Result) bool {
			numItems++
			source := value.Get("_source").String()
			fmt.Println(source)
			return true
		})

		if numItems < batchSize {
			break
		}

		scrollID := gData.Get("_scroll_id").String()
		// 获取下一批结果
		scrollRequest.ScrollID = scrollID

		response, err := scrollRequest.Do(context.Background(), client)
		if err != nil {
			return err
		}
		defer response.Body.Close()

		gData = gjson.Parse(response.String()[9:])
	}

	return
}
