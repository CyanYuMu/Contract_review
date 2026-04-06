import client from "@/utils/client";
import {getAuthToken} from "@/utils/client";
import type {AxiosError} from "axios";
export interface AcceptContractRequest {
    file_id:number;
    is_accepted: boolean;
}
export const contractAccept=async (data:AcceptContractRequest)=>{
    try{
        const token= await getAuthToken();
        const response=await client.post("/review_task/accept_contract_file",data,{
            headers:{
                "Content-Type": "application/json",
                ...(token && {'Authorization': `Bearer ${token}`}),
            }
        })
        return response.data;
    }catch(error){
        const axiosErr = error as AxiosError<{ message?: string; msg?: string }>;
        const message = axiosErr.response?.data?.message || axiosErr.response?.data?.msg || (axiosErr as Error).message || '状态修改失败';
        throw new Error(message);
    }
}