'use client';

import React, { useEffect } from 'react';
import Topbar from '@/components/Topbar';
import LoginModal from '@/components/auth/LoginModal';
import RegisterModal from '@/components/auth/RegisterModal';
import { useAuth } from '@/hooks/useAuth';
import { authDatedHandler } from '@/utils/authDatedHandler';
import type { TabType } from '@/components/TopbarTabs';

interface PageLayoutProps {
    children: React.ReactNode;
    activeTab: TabType;
    showFooter?: boolean;
}

/**
 * 页面布局组件
 * 包含 Topbar、登录/注册模态框和页面内容
 */
export default function PageLayout({ children, activeTab, showFooter = true }: PageLayoutProps) {
    const {
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
    } = useAuth();

    // 注册登录回调，使 403 模态框可以触发登录模态框
    useEffect(() => {
        const unregister = authDatedHandler.registerLoginCallback(() => {
            handleLoginClick();
        });
        return () => {
            unregister();
        };
    }, [handleLoginClick]);

    return (
        <div className="flex h-screen">
            <div className="flex-1 flex flex-col overflow-hidden">
                <Topbar
                    user={user}
                    onLoginClick={handleLoginClick}
                    onLogoutClick={handleLogout}
                    activeTab={activeTab}
                />
                <div className="flex-1 flex flex-col overflow-hidden bg-[#f3f4f6]">
                    {children}

                    {showFooter && (
                        <div className="text-center text-[0.88rem] text-[#4d4d4d] py-4">
                            Copyright ◎2024重庆邮电大学 | 技术支持：信息与网络管理中心、蓝山工作室（联系电话6246 1296）
                        </div>
                    )}
                </div>
            </div>

            <LoginModal
                visible={loginVisible}
                onCancel={() => setLoginVisible(false)}
                onSuccess={handleLoginSuccess}
                onSwitchToRegister={handleSwitchToRegister}
            />

            <RegisterModal
                visible={registerVisible}
                onCancel={() => setRegisterVisible(false)}
                onSuccess={handleRegisterSuccess}
                onSwitchToLogin={handleSwitchToLogin}
            />
        </div>
    );
}
