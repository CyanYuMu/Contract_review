-- =====================================================================
-- 合同分类体系迁移脚本：旧分类 -> 七大类标准分类
-- 标准分类：买卖合同/服务合同/劳动合同/租赁合同/借款合同/合作合同/知识产权合同/通用/其他
--
-- 旧 -> 新 映射：
--   货物类合同 / 基建类合同 / 货物合同 / 基建合同  -> 买卖合同
--   服务类合同 / 服务合同                          -> 服务合同
--   劳动合同 / 劳务合同                            -> 劳动合同
--   租赁合同                                        -> 租赁合同
--   借款合同 / 贷款合同                             -> 借款合同
--   合作合同 / 合伙合同 / 合资合同                  -> 合作合同
--   知识产权合同                                    -> 知识产权合同
--   通用                                            -> 通用（保留）
--   其他 / 未识别                                   -> 其他（保留）
--
-- 本脚本可重复执行（幂等）。
-- =====================================================================

-- 1. 确保七大标准类型 + 通用 + 其他 存在（不存在则插入）
INSERT IGNORE INTO `contract_types` (`name`, `template_content`, `creator`, `created_at`, `updated_at`) VALUES
('买卖合同',     '', 'system', NOW(3), NOW(3)),
('服务合同',     '', 'system', NOW(3), NOW(3)),
('劳动合同',     '', 'system', NOW(3), NOW(3)),
('租赁合同',     '', 'system', NOW(3), NOW(3)),
('借款合同',     '', 'system', NOW(3), NOW(3)),
('合作合同',     '', 'system', NOW(3), NOW(3)),
('知识产权合同', '', 'system', NOW(3), NOW(3)),
('通用',         '', 'system', NOW(3), NOW(3)),
('其他',         '', 'system', NOW(3), NOW(3));

-- 2. 迁移已有合同的 type_id 指向标准类型（先迁移引用，再删除旧类型避免外键冲突）
-- 2.1 货物类/基建类 -> 买卖合同
UPDATE `contracts` c
JOIN `contract_types` t ON c.`type_id` = t.`id`
SET c.`type_id` = (SELECT id FROM `contract_types` WHERE `name` = '买卖合同')
WHERE t.`name` IN ('货物类合同','基建类合同','货物合同','基建合同');

-- 2.2 服务类 -> 服务合同
UPDATE `contracts` c
JOIN `contract_types` t ON c.`type_id` = t.`id`
SET c.`type_id` = (SELECT id FROM `contract_types` WHERE `name` = '服务合同')
WHERE t.`name` = '服务类合同';

-- 2.3 劳务类 -> 劳动合同
UPDATE `contracts` c
JOIN `contract_types` t ON c.`type_id` = t.`id`
SET c.`type_id` = (SELECT id FROM `contract_types` WHERE `name` = '劳动合同')
WHERE t.`name` IN ('劳务合同');

-- 2.4 借款/贷款类 -> 借款合同
UPDATE `contracts` c
JOIN `contract_types` t ON c.`type_id` = t.`id`
SET c.`type_id` = (SELECT id FROM `contract_types` WHERE `name` = '借款合同')
WHERE t.`name` IN ('贷款合同');

-- 2.5 合作/合伙/合资类 -> 合作合同
UPDATE `contracts` c
JOIN `contract_types` t ON c.`type_id` = t.`id`
SET c.`type_id` = (SELECT id FROM `contract_types` WHERE `name` = '合作合同')
WHERE t.`name` IN ('合伙合同','合资合同');

-- 3. 删除已被标准类型取代的旧合同类型（其合同引用已迁移，可安全删除）
DELETE FROM `contract_types`
WHERE `name` IN ('货物类合同','基建类合同','货物合同','基建合同','服务类合同','劳务合同','贷款合同','合伙合同','合资合同');

-- 4. 迁移知识库文档 sub_category（自由文本列，直接 UPDATE）
UPDATE `review_knowledge_docs` SET `sub_category` = '买卖合同'
WHERE `sub_category` IN ('货物类合同','基建类合同','货物合同','基建合同');
UPDATE `review_knowledge_docs` SET `sub_category` = '服务合同'
WHERE `sub_category` = '服务类合同';
UPDATE `review_knowledge_docs` SET `sub_category` = '劳动合同'
WHERE `sub_category` = '劳务合同';
UPDATE `review_knowledge_docs` SET `sub_category` = '借款合同'
WHERE `sub_category` = '贷款合同';
UPDATE `review_knowledge_docs` SET `sub_category` = '合作合同'
WHERE `sub_category` IN ('合伙合同','合资合同');

-- 5. 迁移风险点配置 contract_type_name
UPDATE `review_risk_points` SET `contract_type_name` = '买卖合同'
WHERE `contract_type_name` IN ('货物类合同','基建类合同','货物合同','基建合同');
UPDATE `review_risk_points` SET `contract_type_name` = '服务合同'
WHERE `contract_type_name` = '服务类合同';
UPDATE `review_risk_points` SET `contract_type_name` = '劳动合同'
WHERE `contract_type_name` = '劳务合同';
UPDATE `review_risk_points` SET `contract_type_name` = '借款合同'
WHERE `contract_type_name` = '贷款合同';
UPDATE `review_risk_points` SET `contract_type_name` = '合作合同'
WHERE `contract_type_name` IN ('合伙合同','合资合同');
