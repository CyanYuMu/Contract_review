import client from "@/utils/client";
export const getCasLogin = async () => {
    try{
        const response=await client.get('/login');
        return response.data;
    }catch(error){
        console.log(error);
    }
}