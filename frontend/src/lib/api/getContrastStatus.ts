import client from "@/utils/client";
import type {AxiosError} from "axios";
import toast from "react-hot-toast";
export const getContrastStatus=async(file_id:number)=>{
    try {
        const response=await client.get('/contract/check_file',{
            params:{file_id}
        })
        return response.data.data;
    }catch(err){
        const axiosErr = err as AxiosError<{ message?: string; msg?: string }>;
        const errorMessage = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '获取模型配置失败';
        toast.error(errorMessage);
        throw err;
    }


}
