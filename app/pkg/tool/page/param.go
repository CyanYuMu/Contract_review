package page

import "errors"

type Option struct {
	// 每页最大记录数, 超过认为无效请求, <=0 表示不限制
	MaxPageSize int64
	// 累计最大记录数, <=0 表示不限制
	MaxNumRecord int64
	// 默认每页记录数, 20
	DftPageSize int64
	// 默认页码, 1
	DftPage int64
}

var ErrExceedMaxNumOfPerPage = errors.New("exceeding the maximum number of records per page limit")
var ErrExceedMaxTotalNum = errors.New("exceeding the maximum total number of records limit")

/*Filter
* @Description: page 分页参数过滤器, 限制每页最大页数, 限制总记录数
* @param page
* @param pageSize
* @param maxPageSize
* @param maxNumRecord
* @return pg
* @return ps
* @return ok
 */
func Filter(page, pageSize int64, opt *Option) (pg int64, ps int64, err error) {
	if opt == nil {
		opt = &Option{
			MaxPageSize:  0,
			MaxNumRecord: 0,
			DftPageSize:  20,
			DftPage:      1,
		}
	} else {
		if opt.DftPageSize < 1 {
			opt.DftPageSize = 20
		}
		if opt.DftPage < 1 {
			opt.DftPage = 1
		}
	}

	if page < 1 {
		page = opt.DftPage
	}
	if pageSize < 1 {
		pageSize = opt.DftPage
	}
	if opt.MaxPageSize > 0 && pageSize > opt.MaxPageSize {
		err = ErrExceedMaxNumOfPerPage
		return
	}

	offset := (page - 1) * pageSize
	to := offset + pageSize

	if opt.MaxNumRecord > 0 && to > opt.MaxNumRecord {
		err = ErrExceedMaxTotalNum
		return
	}

	return page, pageSize, nil
}
