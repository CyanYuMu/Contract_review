import client from '@/utils/client';

export const logout = async () => {
    try {
        return await client.post('/user/logout');
    }catch(err) {
        console.error(err);
    }
};
