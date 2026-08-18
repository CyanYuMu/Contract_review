import axios from 'axios';
import { refreshToken } from '@/lib/api/token';
import { authDatedHandler } from '@/utils/authDatedHandler';

// 存储竞态条件处理的状态
let isRefreshing = false;
let refreshSubscribers: ((token: string) => void)[] = [];


export const getAuthToken = (): string | undefined => {
    if (typeof window === 'undefined') return undefined;
    return localStorage.getItem('access_token') || undefined;
};


export const getRefreshToken = (): string | undefined => {
    if (typeof window === 'undefined') return undefined;
    return localStorage.getItem('refresh_token') || undefined;
};

export const getTokenType = (): string => {
    if (typeof window === 'undefined') return 'Bearer';
    return localStorage.getItem('token_type') || 'Bearer';
};

// 保存新的token信息
export const saveTokenInfo = (tokenData: {
    access_token: string;
    token_type?: string;
    refresh_token?: string;
}) => {
    if (typeof window === 'undefined') return;
    
    localStorage.setItem('access_token', tokenData.access_token);
    
    if (tokenData.token_type) {
        localStorage.setItem('token_type', tokenData.token_type);
    }
    
    if (tokenData.refresh_token) {
        localStorage.setItem('refresh_token', tokenData.refresh_token);
    }
};

// 清除所有token和用户缓存
export const clearTokenInfo = () => {
    if (typeof window === 'undefined') return;
    
    localStorage.removeItem('access_token');
    localStorage.removeItem('refresh_token');
    localStorage.removeItem('token_type');
    
    // 清除用户信息缓存
    sessionStorage.removeItem('user_info_cache');
};

// 创建axios实例
const client = axios.create({
    // baseURL: '/api',
    baseURL: '/api/proxy',// 上线时使用该配置
    timeout: 300000, 
    headers: {
        'Content-Type': 'application/json',
    },
});

// 请求拦截器：添加Authorization头
client.interceptors.request.use((config) => {
    const token = getAuthToken();
    if (token) {
        config.headers = {
            ...(config.headers || {}),
            Authorization: `${getTokenType()} ${token}`,
        } as typeof config.headers;
    }
    return config;
});

// 响应拦截器：处理token过期和刷新
client.interceptors.response.use(
    (response) => response,
    async (error) => {
        const originalRequest = error.config;

        // 403 表示"已登录但无权限"（例如 member 访问管理接口），并非登录过期。
        // 不应弹"登录已过期"模态框，直接交给调用方处理（展示无权限提示等）。
        // 只有 401 才代表凭证失效，需要走下方刷新/重新登录逻辑。
        if (error.response?.status !== 401) {
            return Promise.reject(error);
        }
        const requestUrl = originalRequest?.url as string | undefined;
        if (requestUrl?.includes('/user/me')) {
            clearTokenInfo();
            return Promise.reject(error);
        }
        // 如果是401错误，但没有refresh_token，直接清除token并弹出登录过期提示
        const refreshTokenStr = getRefreshToken();
        if (!refreshTokenStr) {
            clearTokenInfo();
            // 触发登录过期模态框（与403共用同一个处理器，确保只弹出一次）
            authDatedHandler.trigger403Error();
            return Promise.reject(error);
        }
        
        // 防止重复刷新token
        if (originalRequest._retry) {
            return Promise.reject(error);
        }
        
        originalRequest._retry = true;
        
        // 如果正在刷新token，将请求加入订阅队列
        if (isRefreshing) {
            return new Promise((resolve) => {
                refreshSubscribers.push((newToken) => {
                    originalRequest.headers.Authorization = `${getTokenType()} ${newToken}`;
                    resolve(client(originalRequest));
                });
            });
        }
        
        isRefreshing = true;
        
        try {
            // 调用刷新token的API
            const newTokenData = await refreshToken(refreshTokenStr);
            
            // 保存新的token信息
            saveTokenInfo({
                access_token: newTokenData.access_token,
                token_type: newTokenData.token_type,
                refresh_token: newTokenData.refresh_token || refreshTokenStr, // 如果返回了新的refresh_token就用新的，否则用旧的
            });
            
            // 更新当前请求的Authorization头
            originalRequest.headers.Authorization = `${getTokenType()} ${newTokenData.access_token}`;
            
            // 通知所有订阅的请求
            refreshSubscribers.forEach((callback) => {
                callback(newTokenData.access_token);
            });
            refreshSubscribers = [];
            
            // 重新发送原始请求
            return client(originalRequest);
        } catch (refreshError) {
            // 刷新token失败，清除所有token并弹出登录过期提示
            clearTokenInfo();
            // 触发登录过期模态框（与403共用同一个处理器，确保只弹出一次）
            authDatedHandler.trigger403Error();
            
            return Promise.reject(refreshError);
        } finally {
            isRefreshing = false;
        }
    }
);

export default client;


