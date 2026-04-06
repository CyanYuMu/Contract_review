'use client';

import { useState, useEffect, useCallback } from 'react';
import { getUserInfo } from '@/lib/api/user';
import { logout } from '@/lib/api/logout';
import { clearTokenInfo } from '@/utils/client';
import type { User, ApiError } from '@/lib/Interface';

/**
 * 用户认证 Hook
 * 提供用户状态、登录/注册模态框控制和认证相关操作
 */
export function useAuth() {
    const [user, setUser] = useState<User | null>(null);
    const [loginVisible, setLoginVisible] = useState(false);
    const [registerVisible, setRegisterVisible] = useState(false);

    useEffect(() => {
        const checkLoginStatus = async () => {
            const token = localStorage.getItem('access_token');
            if (token) {
                try {
                    const userInfo = await getUserInfo();
                    setUser(userInfo);
                } catch (error) {
                    localStorage.removeItem('access_token');
                    const apiError = error as ApiError;
                    if (apiError.status === 401) {
                        setLoginVisible(true);
                    }
                }
            }
        };

        checkLoginStatus();
    }, []);

    const handleLoginSuccess = async (token: string) => {
        try {
            const userInfo = await getUserInfo();
            setUser(userInfo);
            if (token) localStorage.setItem('access_token', token);
        } catch (error) {
            console.warn('获取用户信息失败:', error);
            const apiError = error as ApiError;
            if (apiError.status === 401) {
                setLoginVisible(true);
            }
        }
        setLoginVisible(false);
        // 登录成功后刷新页面
        window.location.reload();
    };

    const handleRegisterSuccess = () => {
        setRegisterVisible(false);
        setLoginVisible(true);
    };

    const handleLoginClick = useCallback(() => {
        setLoginVisible(true);
    }, []);

    const handleSwitchToRegister = () => {
        setLoginVisible(false);
        setRegisterVisible(true);
    };

    const handleSwitchToLogin = () => {
        setRegisterVisible(false);
        setLoginVisible(true);
    };

    const handleLogout = async () => {
        try {
            await logout();
        } catch (error) {
            console.warn('登出失败:', error);
        } finally {
            clearTokenInfo();
            setUser(null);
        }
    };

    return {
        user,
        loginVisible,
        setLoginVisible,
        registerVisible,
        setRegisterVisible,
        handleLoginSuccess,
        handleRegisterSuccess,
        handleLoginClick,
        handleSwitchToRegister,
        handleSwitchToLogin,
        handleLogout,
    };
}
