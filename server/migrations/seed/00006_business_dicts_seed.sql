-- +goose Up
INSERT INTO sys_dict_type (dict_name, dict_code, status, sort_order, is_builtin, remark)
SELECT '商品类型', 'PRODUCT_TYPE', 1, 10, 1, '商品与服务档案类型'
WHERE NOT EXISTS (SELECT 1 FROM sys_dict_type WHERE dict_code = 'PRODUCT_TYPE');

INSERT INTO sys_dict_type (dict_name, dict_code, status, sort_order, is_builtin, remark)
SELECT '往来单位分类', 'PARTNER_TYPE', 1, 11, 1, '往来单位的单位或个人分类'
WHERE NOT EXISTS (SELECT 1 FROM sys_dict_type WHERE dict_code = 'PARTNER_TYPE');

INSERT INTO sys_dict_type (dict_name, dict_code, status, sort_order, is_builtin, remark)
SELECT '往来单位身份', 'PARTNER_IDENTITY', 1, 12, 1, '往来单位可同时具有客户与供应商身份'
WHERE NOT EXISTS (SELECT 1 FROM sys_dict_type WHERE dict_code = 'PARTNER_IDENTITY');

INSERT INTO sys_dict_type (dict_name, dict_code, status, sort_order, is_builtin, remark)
SELECT '业务档案状态', 'BUSINESS_STATUS', 1, 13, 1, '商品、往来单位的启用与停用状态'
WHERE NOT EXISTS (SELECT 1 FROM sys_dict_type WHERE dict_code = 'BUSINESS_STATUS');

INSERT INTO sys_dict_data (dict_type_id, dict_label, dict_value, sort_order)
SELECT t.id, '实物', 'GOODS', 1 FROM sys_dict_type t
WHERE t.dict_code = 'PRODUCT_TYPE' AND t.is_builtin = 1 AND NOT EXISTS
  (SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id = t.id AND d.dict_value = 'GOODS' AND d.deleted = 0);
INSERT INTO sys_dict_data (dict_type_id, dict_label, dict_value, sort_order)
SELECT t.id, '服务', 'SERVICE', 2 FROM sys_dict_type t
WHERE t.dict_code = 'PRODUCT_TYPE' AND t.is_builtin = 1 AND NOT EXISTS
  (SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id = t.id AND d.dict_value = 'SERVICE' AND d.deleted = 0);

INSERT INTO sys_dict_data (dict_type_id, dict_label, dict_value, sort_order)
SELECT t.id, '单位', 'COMPANY', 1 FROM sys_dict_type t
WHERE t.dict_code = 'PARTNER_TYPE' AND t.is_builtin = 1 AND NOT EXISTS
  (SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id = t.id AND d.dict_value = 'COMPANY' AND d.deleted = 0);
INSERT INTO sys_dict_data (dict_type_id, dict_label, dict_value, sort_order)
SELECT t.id, '个人', 'PERSON', 2 FROM sys_dict_type t
WHERE t.dict_code = 'PARTNER_TYPE' AND t.is_builtin = 1 AND NOT EXISTS
  (SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id = t.id AND d.dict_value = 'PERSON' AND d.deleted = 0);

INSERT INTO sys_dict_data (dict_type_id, dict_label, dict_value, sort_order)
SELECT t.id, '客户', 'CUSTOMER', 1 FROM sys_dict_type t
WHERE t.dict_code = 'PARTNER_IDENTITY' AND t.is_builtin = 1 AND NOT EXISTS
  (SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id = t.id AND d.dict_value = 'CUSTOMER' AND d.deleted = 0);
INSERT INTO sys_dict_data (dict_type_id, dict_label, dict_value, sort_order)
SELECT t.id, '供应商', 'SUPPLIER', 2 FROM sys_dict_type t
WHERE t.dict_code = 'PARTNER_IDENTITY' AND t.is_builtin = 1 AND NOT EXISTS
  (SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id = t.id AND d.dict_value = 'SUPPLIER' AND d.deleted = 0);

INSERT INTO sys_dict_data (dict_type_id, dict_label, dict_value, sort_order)
SELECT t.id, '启用', '1', 1 FROM sys_dict_type t
WHERE t.dict_code = 'BUSINESS_STATUS' AND t.is_builtin = 1 AND NOT EXISTS
  (SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id = t.id AND d.dict_value = '1' AND d.deleted = 0);
INSERT INTO sys_dict_data (dict_type_id, dict_label, dict_value, sort_order)
SELECT t.id, '停用', '0', 2 FROM sys_dict_type t
WHERE t.dict_code = 'BUSINESS_STATUS' AND t.is_builtin = 1 AND NOT EXISTS
  (SELECT 1 FROM sys_dict_data d WHERE d.dict_type_id = t.id AND d.dict_value = '0' AND d.deleted = 0);

-- +goose Down
DELETE FROM sys_dict_data
WHERE dict_type_id IN (SELECT id FROM sys_dict_type WHERE dict_code IN ('PRODUCT_TYPE', 'PARTNER_TYPE', 'PARTNER_IDENTITY', 'BUSINESS_STATUS'))
  AND dict_value IN ('GOODS', 'SERVICE', 'COMPANY', 'PERSON', 'CUSTOMER', 'SUPPLIER', '1', '0')
  AND EXISTS (SELECT 1 FROM sys_dict_type t WHERE t.id = sys_dict_data.dict_type_id AND t.is_builtin = 1);
DELETE FROM sys_dict_type
WHERE dict_code IN ('PRODUCT_TYPE', 'PARTNER_TYPE', 'PARTNER_IDENTITY', 'BUSINESS_STATUS')
  AND is_builtin = 1
  AND NOT EXISTS (SELECT 1 FROM sys_dict_data WHERE dict_type_id = sys_dict_type.id);
