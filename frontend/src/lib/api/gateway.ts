import client from '@/utils/client';
import type {AxiosError} from 'axios';
import type {UsageStat, DailyUsageTrend, GatewayRoute, LLMQuota} from '@/lib/Interface';

/**
 * 解包网关接口响应：后端统一返回 {code,msg,data}，取 data 字段。
 * 失败时抛出后端 msg。
 */
const unwrap = async <T>(p: Promise<{data: {code?: number; msg?: string; data?: unknown}}>): Promise<T> => {
    try {
        const res = await p;
        return res.data?.data as T;
    } catch (error) {
        const axiosErr = error as AxiosError<{msg?: string}>;
        throw new Error(axiosErr.response?.data?.msg || '请求失败');
    }
};

/** 用量统计 GET /gateway/usage_stats?dimension=feature|user&days= */
export const getUsageStats = (dimension: 'feature' | 'user', days = 30) =>
    unwrap<UsageStat[]>(client.get('/gateway/usage_stats', {params: {dimension, days}}));

/** 用量趋势 GET /gateway/usage_trend?days= */
export const getUsageTrend = (days = 30) =>
    unwrap<DailyUsageTrend[]>(client.get('/gateway/usage_trend', {params: {days}}));

/** 路由列表 GET /gateway/routes */
export const listGatewayRoutes = () => unwrap<GatewayRoute[]>(client.get('/gateway/routes'));

/** 更新路由（切换某功能所用模型，不动应用代码）PUT /gateway/route */
export const updateGatewayRoute = (data: {
    feature: string;
    model_config_id: number;
    params?: string;
}) => unwrap<void>(client.put('/gateway/route', data));

/** 查询用户配额 GET /gateway/quotas?user_id= */
export const listQuotas = (userId: number) =>
    unwrap<LLMQuota[]>(client.get('/gateway/quotas', {params: {user_id: userId}}));

/** 设置配额 PUT /gateway/quota */
export const setQuota = (data: LLMQuota) => unwrap<LLMQuota>(client.put('/gateway/quota', data));
