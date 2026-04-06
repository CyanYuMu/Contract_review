import client from '@/utils/client'
import { AxiosError } from 'axios'

const fetchStatistic = async (url: string) => {
  try {
    const response = await client.get(url);
    return response.data;
  } catch (error) {
    const axiosErr = error as AxiosError<{ message: string }>;
    const message = axiosErr.response?.data?.message || '获取数据失败，请稍后重试';
    throw new Error(message);
  }
};


export const getContractReviewOverview = () => fetchStatistic('/signboard/statistics_overview');
export const getRevisionStatistics = () => fetchStatistic('/signboard/statistics_revisions');
export const getDepartmentUsage=()=>fetchStatistic('/signboard/departments_usage')
export const getYesterdayContractData=()=>fetchStatistic('/signboard/trends_contracts')