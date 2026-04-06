import toast from "react-hot-toast";
import client, {getAuthToken} from "@/utils/client";
import {PromptListParams} from "@/lib/Interface";
import type {AxiosError} from "axios";

export const getOrganizationPrompt = async (params: PromptListParams) => {
    try {
        const queryParams = {
            contract_type_id: params.contract_type_id,
            organization_id: params.organization_id,
        };
        const token = getAuthToken();
        const res = await client.get('/prompt_manage/org/', {
            params: queryParams,
            headers: token ? {Authorization: `Bearer ${token}`} : undefined,
        });
        const promptText = res.data?.data?.prompt_text;
        return promptText as string;
    } catch (err) {
        const axiosErr = err as AxiosError<{ message?: string; msg?: string }>;
        const errorMessage = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '获取模型配置失败';
        toast.error(errorMessage);
        throw err;
    }
}
