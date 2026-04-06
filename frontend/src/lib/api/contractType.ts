import client from "@/utils/client";

// ============ 类型定义 ============

// 合同类型创建者
export type ContractTypeCreator = {
    id: string;
    name: string;
};

// 合同类型分页查询请求参数
export type ContractTypePageRequest = {
    contractTypeName?: string;  // 合同类型名称（模糊查询）
    creator?: string;           // 创建人（模糊查询）
    startDate?: string;         // 开始日期，格式 YYYY-MM-DD
    endDate?: string;           // 结束日期，格式 YYYY-MM-DD
    page: number;               // 当前页码，从1开始
    pageSize: number;           // 每页条数，1-100
};

// 合同类型列表项
export type ContractTypeListItem = {
    id: string;
    contractTypeName: string;
    templateContent?: string | null;
    creator: string;
    updateDate: string;
};

// 合同类型分页查询响应
export type ContractTypePageResponse = {
    list: ContractTypeListItem[];
    total: number;
    page: number;
    pageSize: number;
};

// 合同类型详情响应
export type ContractTypeDetailResponse = {
    id: number;
    name: string;
    template_content?: string | null;
    creator_id?: number | null;
    is_active: number;
    create_time: string;
    update_time: string;
};

// 创建合同类型请求参数
export type ContractTypeCreateRequest = {
    contractTypeName: string;   // 合同类型名称，最大长度50字符
    templateContent: string;    // 提示词模板内容，最大长度5000字符
};

// 编辑合同类型请求参数
export type ContractTypeUpdateRequest = {
    contractTypeName: string;   // 合同类型名称，最大长度50字符
    templateContent: string;    // 提示词模板内容，最大长度5000字符
};

// 批量删除请求参数
export type ContractTypeBatchDeleteRequest = {
    ids: number[];              // 合同类型ID列表
};

// ============ API 接口 ============

/**
 * 获取合同类型创建者列表
 * 获取系统中所有创建过合同类型的用户列表，用于筛选创建人
 */
export const getContractTypeCreators = async () => {
    const response = await client.get('/contract_type/creators');
    return response.data;
};

/**
 * 获取合同类型分页列表
 * 分页查询合同类型列表，支持多条件筛选
 */
export const getContractTypePage = async (params: ContractTypePageRequest) => {
    const response = await client.get('/contract_type/page', { params });
    return response.data;
};

/**
 * 获取所有激活的合同类型列表
 */
export const getContractTypeList = async () => {
    const response = await client.get('/contract_type/list');
    return response.data;
};

/**
 * 获取合同类型详情
 * @param contractTypeId 合同类型ID
 */
export const getContractTypeDetail = async (contractTypeId: number) => {
    const response = await client.get(`/contract_type/detail/${contractTypeId}`);
    return response.data;
};

/**
 * 创建合同类型
 */
export const createContractType = async (data: ContractTypeCreateRequest) => {
    const response = await client.post('/contract_type/create', data);
    return response.data;
};

/**
 * 编辑合同类型
 * @param contractTypeId 合同类型ID
 * @param data 更新数据
 */
export const updateContractType = async (contractTypeId: number, data: ContractTypeUpdateRequest) => {
    const response = await client.put(`/contract_type/update/${contractTypeId}`, data);
    return response.data;
};

/**
 * 删除合同类型
 * @param contractTypeId 合同类型ID
 */
export const deleteContractType = async (contractTypeId: number) => {
    const response = await client.delete(`/contract_type/delete/${contractTypeId}`);
    return response.data;
};

/**
 * 批量删除合同类型
 * @param ids 合同类型ID列表
 */
export const batchDeleteContractType = async (ids: number[]) => {
    const response = await client.delete('/contract_type/batchdelete', { data: { ids } });
    return response.data;
};
