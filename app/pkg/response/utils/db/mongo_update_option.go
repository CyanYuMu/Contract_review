package db

type BatchUpdateOpt struct {
	FieldOpt    *Opt
	IgnoreError *bool
}

// apply
func (o *BatchUpdateOpt) Apply(opt *BatchUpdateOpt) {
	if opt == nil {
		return
	}
	if opt.FieldOpt != nil {
		if opt.FieldOpt.EmptyIgnore != nil {
			o.FieldOpt.EmptyIgnore = opt.FieldOpt.EmptyIgnore
		}
		if opt.FieldOpt.Tag != "" {
			o.FieldOpt.Tag = opt.FieldOpt.Tag
		}
		if len(opt.FieldOpt.FieldOpRef) > 0 {
			o.FieldOpt.FieldOpRef = opt.FieldOpt.FieldOpRef
		}
	}
	if opt.IgnoreError != nil {
		o.IgnoreError = opt.IgnoreError
	}
}

var dftBatchUpdateOpt = func() *BatchUpdateOpt {
	var ignore = false
	opt := &BatchUpdateOpt{
		FieldOpt: &Opt{
			EmptyIgnore:           nil,
			Tag:                   "bson",
			IgnoreUpdateTimeField: false,
			FieldOpRef:            map[string]FieldOpFunc{"-": KeyOpSet},
		},
		IgnoreError: &ignore,
	}

	return opt
}
