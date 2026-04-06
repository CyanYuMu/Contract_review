package knowledge

import "gorm.io/gorm"

// AutoMigrate 创建/更新审阅知识库表（应用启动时可调用一次）
func AutoMigrate(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	return db.AutoMigrate(&ReviewKnowledgeDoc{}, &ReviewKnowledgeChunk{})
}
