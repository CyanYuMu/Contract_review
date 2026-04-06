import client from "@/utils/client";
export const getHistoryDetail = async (session_id:string) => {
    try {
        const response = await client.post('/session/session_history_detail', {
            session_id,
        });
        return response.data;
    }catch(err){
        console.error(err);
    }
}
