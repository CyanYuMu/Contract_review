-- 审阅规范 / 法规知识库（与 app/internal/knowledge 模型一致）
-- 在目标库执行；若使用 GORM AutoMigrate 可仅执行下方 INSERT 做种子数据。

CREATE TABLE IF NOT EXISTS `review_knowledge_docs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `title` varchar(255) NOT NULL COMMENT '标题',
  `category` varchar(16) NOT NULL COMMENT '规范/法规/案例/示范',
  `sub_category` varchar(64) DEFAULT NULL COMMENT '合同类型子类',
  `source` varchar(255) DEFAULT NULL COMMENT '来源',
  `content` longtext COMMENT '全文',
  `chunk_count` bigint DEFAULT '0' COMMENT '分块数',
  `status` varchar(16) DEFAULT 'pending' COMMENT 'pending/indexed/failed',
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_review_knowledge_docs_category` (`category`),
  KEY `idx_review_knowledge_docs_sub_category` (`sub_category`),
  KEY `idx_review_knowledge_docs_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='审阅知识库文档';

CREATE TABLE IF NOT EXISTS `review_knowledge_chunks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `doc_id` bigint unsigned NOT NULL COMMENT '所属文档',
  `chunk_index` bigint NOT NULL COMMENT '分块序号',
  `content` longtext NOT NULL COMMENT '分块文本',
  `vector_id` varchar(128) DEFAULT NULL COMMENT '向量库ID(Milvus等)',
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_review_knowledge_chunks_doc_id` (`doc_id`),
  CONSTRAINT `fk_review_knowledge_chunks_doc` FOREIGN KEY (`doc_id`) REFERENCES `review_knowledge_docs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='审阅知识库分块';

-- 示例种子：一条规范 + 一条法规（indexed 才会被 RAG 加载）
INSERT INTO `review_knowledge_docs`
  (`title`, `category`, `sub_category`, `source`, `content`, `chunk_count`, `status`, `created_at`, `updated_at`)
VALUES
(
  '服务类合同审阅要点（示例）',
  '规范',
  '服务类合同',
  '内部审阅指引',
  '服务类合同中应重点审查：服务范围是否明确、验收标准、价款与支付节点、知识产权归属、保密义务、违约责任与解除条件。单方解除条款应平衡双方利益，避免显失公平。',
  1,
  'indexed',
  NOW(3),
  NOW(3)
),
(
  '民法典合同编（摘录示例）',
  '法规',
  '通用',
  '《中华人民共和国民法典》',
  '当事人一方不履行合同义务或者履行合同义务不符合约定的，应当承担继续履行、采取补救措施或者赔偿损失等违约责任。约定的违约金低于造成的损失的，人民法院或者仲裁机构可以根据当事人的请求予以增加。',
  1,
  'indexed',
  NOW(3),
  NOW(3)
);

INSERT INTO `review_knowledge_chunks` (`doc_id`, `chunk_index`, `content`, `created_at`)
SELECT `id`, 0, `content`, NOW(3) FROM `review_knowledge_docs` WHERE `title` = '服务类合同审阅要点（示例）';

INSERT INTO `review_knowledge_chunks` (`doc_id`, `chunk_index`, `content`, `created_at`)
SELECT `id`, 0, `content`, NOW(3) FROM `review_knowledge_docs` WHERE `title` = '民法典合同编（摘录示例）';
