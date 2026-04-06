package dualwrite

// Config 双写配置
type Config struct {
	Enabled bool `yaml:"enabled"` // 双写总开关，false 时只使用主客户端，不双写
	// 增量同步是否在运行：true=只双写幂等命令，false=双写所有写命令
	SyncRunning bool `yaml:"syncRunning"`
	// 主客户端选择（决定所有读写操作走哪个库）：
	// "old" 或 空 = 主客户端连接旧库，Enabled=true 时写操作双写到新库
	// "new" = 主客户端连接新库，Enabled=true 时写操作双写到旧库
	ReadFrom string `yaml:"readFrom"`
}
