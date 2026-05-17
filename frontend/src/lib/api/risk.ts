import client from "@/utils/client";

export type RiskPointListItem = {
    id: number;
    key: string;
    riskId: string;
    riskContent: string;
    riskType: string;
    riskLevel: string;
    contractTypeId: number;
    contractType: string;
    applicableContractType: string;
    applicableScope: "individual" | "department" | "platform";
    department: string[];
    creator: string;
    status: string;
    isEnabled: "enabled" | "disabled";
    knowledgeDocId: number;
    ragStatus: string;
    updateDate: string;
    createdAt: string;
};

export type RiskPointPageRequest = {
    riskId?: string;
    riskContent?: string;
    status?: string;
    contractType?: string;
    creator?: string;
    startDate?: string;
    endDate?: string;
    page: number;
    pageSize: number;
};

export type RiskPointSaveRequest = {
    contractTypeId: number;
    contractType?: string;
    applicableScope: "individual" | "department" | "platform";
    department: string[];
    riskContent: string;
    riskType: string;
    riskLevel: string;
    isEnabled: "enabled" | "disabled";
};

export type RiskPointStats = {
    total: number;
    enabled: number;
    disabled: number;
    indexed: number;
    byLevel: Array<{ name: string; value: number }>;
    byType: Array<{ name: string; value: number }>;
    byContractType: Array<{ name: string; value: number }>;
};

export const getRiskPointPage = async (params: RiskPointPageRequest) => {
    const response = await client.get("/risk_point/page", { params });
    return response.data;
};

export const getRiskPointStats = async (contractType?: string) => {
    const response = await client.get("/risk_point/stats", {
        params: contractType ? { contractType } : undefined,
    });
    return response.data;
};

export const createRiskPoint = async (data: RiskPointSaveRequest) => {
    const response = await client.post("/risk_point/create", data);
    return response.data;
};

export const updateRiskPoint = async (id: number, data: RiskPointSaveRequest) => {
    const response = await client.put(`/risk_point/update/${id}`, data);
    return response.data;
};

export const deleteRiskPoint = async (id: number) => {
    const response = await client.delete(`/risk_point/delete/${id}`);
    return response.data;
};

export const batchDeleteRiskPoint = async (ids: number[]) => {
    const response = await client.delete("/risk_point/batchdelete", { data: { ids } });
    return response.data;
};
