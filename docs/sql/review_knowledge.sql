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
  `metadata` json DEFAULT NULL COMMENT '结构化元数据(风险点字段等)',
  `parent_chunk_id` varchar(128) DEFAULT NULL COMMENT '父分块ID(父子分块)',
  `chunk_type` varchar(16) DEFAULT NULL COMMENT 'child/parent',
  `created_at` datetime(3) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_review_knowledge_chunks_doc_id` (`doc_id`),
  KEY `idx_review_knowledge_chunks_parent` (`parent_chunk_id`),
  CONSTRAINT `fk_review_knowledge_chunks_doc` FOREIGN KEY (`doc_id`) REFERENCES `review_knowledge_docs` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='审阅知识库分块';

-- 已存在分块表（老库）需要补充列时，手动执行下面这些（新库无需，CREATE TABLE 已包含）。
-- 应用启动时 GORM AutoMigrate 也会自动补列；此条仅供手工维护数据库的部署参考。
-- ALTER TABLE `review_knowledge_chunks`
--   ADD COLUMN `metadata` json DEFAULT NULL COMMENT '结构化元数据(风险点字段等)' AFTER `vector_id`,
--   ADD COLUMN `parent_chunk_id` varchar(128) DEFAULT NULL COMMENT '父分块ID(父子分块)' AFTER `metadata`,
--   ADD COLUMN `chunk_type` varchar(16) DEFAULT NULL COMMENT 'child/parent' AFTER `parent_chunk_id`;

-- 示例种子：一条规范 + 一条法规（indexed 才会被 RAG 加载）
INSERT INTO `review_knowledge_docs`
  (`title`, `category`, `sub_category`, `source`, `content`, `chunk_count`, `status`, `created_at`, `updated_at`)
VALUES
(
  '服务合同审阅要点（示例）',
  '规范',
  '服务合同',
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
SELECT `id`, 0, `content`, NOW(3) FROM `review_knowledge_docs` WHERE `title` = '服务合同审阅要点（示例）';

INSERT INTO `review_knowledge_chunks` (`doc_id`, `chunk_index`, `content`, `created_at`)
SELECT `id`, 0, `content`, NOW(3) FROM `review_knowledge_docs` WHERE `title` = '民法典合同编（摘录示例）';

-- 更完整的通用审阅知识种子：用于提升中文关键词 RAG 召回。
-- 可重复执行；通过 title 去重插入。
INSERT INTO `review_knowledge_docs`
  (`title`, `category`, `sub_category`, `source`, `content`, `chunk_count`, `status`, `created_at`, `updated_at`)
SELECT * FROM (
  SELECT
    '通用合同核心条款审阅指引' AS `title`,
    '规范' AS `category`,
    '通用' AS `sub_category`,
    '内部审阅指引' AS `source`,
    '合同应明确合同主体、标的或服务范围、交付成果、验收标准、付款节点、发票义务、违约责任、解除终止、保密、知识产权、争议解决和管辖。缺少核心条款或表述不清，会导致履约、验收、付款和争议解决风险。' AS `content`,
    1 AS `chunk_count`,
    'indexed' AS `status`,
    NOW(3) AS `created_at`,
    NOW(3) AS `updated_at`
  UNION ALL SELECT
    '服务合同审阅指引',
    '规范',
    '服务合同',
    '内部审阅指引',
    '服务类合同应重点审查服务内容是否具体、交付物是否可验收、服务期限和里程碑是否明确、质量标准和整改期限是否清楚、费用支付是否与验收挂钩、成果知识产权归属是否明确。仅以附件笼统描述服务内容或缺少验收标准，容易产生争议。',
    1,
    'indexed',
    NOW(3),
    NOW(3)
  UNION ALL SELECT
    '买卖合同审阅指引',
    '规范',
    '买卖合同',
    '内部审阅指引',
    '买卖合同应重点审查标的物名称规格数量质量是否明确、所有权与风险转移时点、交付时间地点方式、验收标准与异议期限、价款支付节点、质量瑕疵担保与保修、逾期交货/付款违约责任、所有权保留条款及解除条件。',
    1,
    'indexed',
    NOW(3),
    NOW(3)
  UNION ALL SELECT
    '劳动合同审阅指引',
    '规范',
    '劳动合同',
    '内部审阅指引',
    '劳动合同应重点审查用工形式、合同期限与试用期是否匹配法定上限、工作内容地点、工时制与加班费、工资构成与试用期工资下限、社保缴纳义务、竞业限制范围期限与补偿金、服务期违约金、解除终止情形与经济补偿。不得约定押金担保扣证或违法违约金。',
    1,
    'indexed',
    NOW(3),
    NOW(3)
  UNION ALL SELECT
    '租赁合同审阅指引',
    '规范',
    '租赁合同',
    '内部审阅指引',
    '租赁合同应重点审查租赁物特定化、租赁期限（动产≤20年）、租金与押金及退还、交付与维修义务划分、按约定用途使用与转租限制、添附改良归属与恢复原状、风险负担、违约责任、优先购买权及期满返还与续租。',
    1,
    'indexed',
    NOW(3),
    NOW(3)
  UNION ALL SELECT
    '借款合同审阅指引',
    '规范',
    '借款合同',
    '内部审阅指引',
    '借款合同应重点审查借款主体、金额币种、用途与挪用限制、利率上限（≤LPR四倍）、还款方式与提前还款、担保范围与登记要件、自然人借款自交付生效、逾期罚息与加速到期、实现债权费用承担。',
    1,
    'indexed',
    NOW(3),
    NOW(3)
  UNION ALL SELECT
    '合作合同审阅指引',
    '规范',
    '合作合同',
    '内部审阅指引',
    '合作合同应重点审查各方出资与作价、合作目标与分工、收益分配与风险分担、重大事项决策与僵局破解、合作前后知识产权归属与许可、保密与竞业、违约责任（出资违约/擅自退出）、退出解散与清算分配。',
    1,
    'indexed',
    NOW(3),
    NOW(3)
  UNION ALL SELECT
    '知识产权合同审阅指引',
    '规范',
    '知识产权合同',
    '内部审阅指引',
    '知识产权合同应重点审查合同类型（转让/许可/开发）、标的权属状态与共有人同意、许可范围（独占/排他/普通/地域/期限/转许可）、对价与权属变更挂钩、后续改进与衍生归属、权利瑕疵担保与侵权诉讼主导权、权属登记备案要件。',
    1,
    'indexed',
    NOW(3),
    NOW(3)
  UNION ALL SELECT
    '违约责任审阅指引',
    '规范',
    '通用',
    '内部审阅指引',
    '违约责任应覆盖逾期付款、逾期交付、质量不合格、拒不整改、擅自解除、泄密、知识产权侵权等主要违约场景。违约金、损失赔偿、继续履行和解除权应与主要义务对应。仅约定一方责任、责任过轻或责任缺失，会对相对方不利。',
    1,
    'indexed',
    NOW(3),
    NOW(3)
  UNION ALL SELECT
    '知识产权与保密审阅指引',
    '规范',
    '通用',
    '内部审阅指引',
    '涉及软件、网站、设计、文案、数据、方案等成果的合同，应明确成果知识产权归属、使用范围、第三方侵权责任、源文件或交付材料范围。保密条款应明确保密信息范围、保密期限、例外情形和违约责任。',
    1,
    'indexed',
    NOW(3),
    NOW(3)
  UNION ALL SELECT
    '争议解决条款审阅指引',
    '规范',
    '通用',
    '内部审阅指引',
    '争议解决条款应明确适用法律、管辖法院或仲裁机构。约定仲裁时应写明准确的仲裁委员会名称；约定诉讼时应明确有管辖连接点的法院。管辖地不明、仲裁机构不存在或同时约定诉讼和仲裁，可能影响条款效力并增加维权成本。',
    1,
    'indexed',
    NOW(3),
    NOW(3)
) AS seed
WHERE NOT EXISTS (
  SELECT 1 FROM `review_knowledge_docs` d WHERE d.`title` = seed.`title`
);

INSERT INTO `review_knowledge_chunks` (`doc_id`, `chunk_index`, `content`, `created_at`)
SELECT d.`id`, 0, d.`content`, NOW(3)
FROM `review_knowledge_docs` d
LEFT JOIN `review_knowledge_chunks` c ON c.`doc_id` = d.`id` AND c.`chunk_index` = 0
WHERE d.`title` IN (
  '通用合同核心条款审阅指引',
  '服务合同审阅指引',
  '买卖合同审阅指引',
  '劳动合同审阅指引',
  '租赁合同审阅指引',
  '借款合同审阅指引',
  '合作合同审阅指引',
  '知识产权合同审阅指引',
  '违约责任审阅指引',
  '知识产权与保密审阅指引',
  '争议解决条款审阅指引'
) AND c.`id` IS NULL;
