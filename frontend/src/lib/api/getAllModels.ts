import toast from "react-hot-toast";
import client, {getAuthToken} from "@/utils/client";
import {ModelsListParams} from "@/lib/Interface";
import type {AxiosError} from "axios";

export const getAllModels = async (params: ModelsListParams = {}) => {
    try {
        const queryParams = {
            page: params.page ?? 1,
            size: params.size ?? 10,
        };
        const token = getAuthToken();
        const res = await client.get('/model_configs/get_all_models/', {
            params: queryParams,
            headers: token ? {Authorization: `Bearer ${token}`} : undefined,
        });
        if (res.data && res.data.data) {
            return res.data.data;
        }
        return res.data;
    } catch (err) {
        const axiosErr = err as AxiosError<{ message?: string; msg?: string }>;
        const errorMessage = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '获取模型配置失败';
        toast.error(errorMessage);
        throw err;
    }
}
