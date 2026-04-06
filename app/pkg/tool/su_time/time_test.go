package su_time

import (
	"fmt"
	"testing"
	"time"
)

func TestTimeUnixToDateDayFile(t *testing.T) {
	s := TimeUnixToDateDayFile()
	fmt.Println(s)
}

func TestConvertToTimezone(t *testing.T) {
	t1 := time.Now()
	loc, err := ConvertToTimezone(t1, "UTC")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(loc, loc.Hour())
}
