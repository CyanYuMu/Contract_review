'use client';

import React, { useCallback, useEffect, useState } from 'react';
import Topbar from "@/components/Topbar";
import type { User } from "@/lib/Interface";
import { Menu, Spin } from 'antd'
import { useRouter, usePathname } from 'next/navigation'
import { getUserInfo } from "@/lib/api/user";
import { authDatedHandler } from '@/utils/authDatedHandler';
import LoginModal from '@/components/auth/LoginModal';
import RegisterModal from '@/components/auth/RegisterModal';
import { saveTokenInfo } from '@/utils/client';

// SVG 图标组件
const ModelIcon = ({ active }: { active: boolean }) => (
    <svg xmlns="http://www.w3.org/2000/svg"
        width="19.5001220703125" height="19.4317626953125"
        viewBox="0 0 19.5001220703125 19.4317626953125" fill="none">
        <path
            d="M18.0087 1.42311C19.8876 3.30204 17.698 8.53808 13.118 13.118C8.53808 17.698 3.30209 19.8876 1.42311 18.0087C-0.45585 16.1297 1.73375 10.8937 6.31371 6.31371C10.8937 1.73375 16.1297 -0.45585 18.0087 1.42311Z"
            stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linejoin="round"
            stroke-linecap="round"></path>
        <path
            d="M1.49147 1.42311C-0.387491 3.30204 1.80211 8.53808 6.38207 13.118C10.962 17.698 16.198 19.8876 18.077 18.0087C19.956 16.1297 17.7664 10.8937 13.1864 6.31371C8.60644 1.73375 3.37044 -0.45585 1.49147 1.42311Z"
            stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linejoin="round"
            stroke-linecap="round"></path>
    </svg>
);

const RiskIcon = ({ active }: { active: boolean }) => (
    <svg xmlns="http://www.w3.org/2000/svg"
        height="21.5" viewBox="0 0 19.5 21.5" fill="none">
        <path stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linejoin="round"
            stroke-linecap="round"
            d="M16.75 6.94885L16.75 1.75C16.75 1.19772 16.3023 0.75 15.75 0.75L1.75 0.75C1.19772 0.75 0.75 1.19772 0.75 1.75L0.75 19.75C0.75 20.3023 1.19772 20.75 1.75 20.75L6.75 20.75"></path>
        <path stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linecap="round"
            d="M4.75 5.75L11.25 5.75"></path>
        <path stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linecap="round"
            d="M4.75 9.25L7.25 9.25"></path>
        <circle cx="23.75" cy="15.75" transform="rotate(90 18.75 10.75)" r="5"
            stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linejoin="round"
            stroke-linecap="round"></circle>
        <path stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linecap="round"
            d="M13.75 16.75L13.75 18.25"></path>
        <circle cx="13.75" cy="13.75" r="1" fill={active ? '#2260F2' : '#383838'} ></circle>
    </svg>
);

const PermissionIcon = ({ active }: { active: boolean }) => (
    <svg xmlns="http://www.w3.org/2000/svg"
        width="19.5" height="19.5" viewBox="0 0 19.5 19.5" fill="none">
        <path stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round"
            d="M1.5 3.75L1.5 1.5C1.5 1.08579 1.83579 0.75 2.25 0.75L17.25 0.75C17.6642 0.75 18 1.08579 18 1.5L18 17.25C18 17.6642 17.6642 18 17.25 18L2.25 18C1.83579 18 1.5 17.6642 1.5 17.25L1.5 15.75"></path>
        <path stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinecap="round"
            d="M5.25 5.25L14.25 5.25"></path>
        <path stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinecap="round"
            d="M5.25 9L10.5 9"></path>
        <path stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinecap="round"
            d="M5.25 12.75L8.25 12.75"></path>
        <circle cx="4.5" cy="9.75" r="4" stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5"></circle>
        <path stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinecap="round"
            d="M4.5 8.25L4.5 10.5L6 10.5"></path>
    </svg>
);

const RoleIcon = ({ active }: { active: boolean }) => (
    <svg xmlns="http://www.w3.org/2000/svg"
        width="19.5003662109375" height="15" viewBox="0 0 19.5003662109375 15" fill="none">
        <path stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round"
            d="M7.95 0.75L1.65 0.75C1.15294 0.75 0.75 1.15293 0.75 1.65L0.75 13.35C0.75 13.8471 1.15294 14.25 1.65 14.25L17.85 14.25C18.3471 14.25 18.75 13.8471 18.75 13.35L18.75 12.225"></path>
        <path stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinecap="round"
            d="M3.4502 6.59998L7.0502 6.59998"></path>
        <path stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinecap="round" d="M3.4502 10.2L14.2502 10.2"></path>
        <ellipse cx="14.249805450439453" cy="3.4499998092651367" rx="2.700000762939453"
            ry="2.6999998092651367" stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinejoin="round"
            strokeLinecap="round"></ellipse>
        <path stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round"
            d="M16.95 8.55L11.55 8.55"></path>
    </svg>
);

const FeedbackIcon = ({ active }: { active: boolean }) => (
   <svg xmlns="http://www.w3.org/2000/svg"
    width="19.50006103515625" height="24" viewBox="0 0 19.50006103515625 24" fill="none">
    <path stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linejoin="round"
        stroke-linecap="round"
        d="M18.75 17.0625L18.75 22.125C18.75 22.7463 18.2463 23.25 17.625 23.25L13.9688 23.25"></path>
    <path stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linejoin="round"
        stroke-linecap="round"
        d="M18.75 7.5L18.75 1.875C18.75 1.25368 18.2463 0.75 17.625 0.75L1.875 0.75C1.25368 0.75 0.75 1.25368 0.75 1.875L0.75 22.125C0.75 22.7463 1.25368 23.25 1.875 23.25L5.25 23.25"></path>
    <path stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linecap="round"
        d="M5.25 7.5L13.125 7.5"></path>
    <path stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linecap="round"
        d="M9.1875 23.25L18.75 11.4375"></path>
    <path stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linecap="round"
        d="M5.25 12L9.75 12"></path>
</svg>
);

const AboutIcon = ({ active }: { active: boolean }) => (
  <svg xmlns="http://www.w3.org/2000/svg"
    height="19.5" viewBox="0 0 19.5 19.5" fill="none">
    <path
        d="M9.75 18.75C14.7206 18.75 18.75 14.7206 18.75 9.75C18.75 4.77944 14.7206 0.75 9.75 0.75C4.77944 0.75 0.75 4.77944 0.75 9.75C0.75 14.7206 4.77944 18.75 9.75 18.75Z"
        stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linejoin="round"
        stroke-linecap="round"></path>
    <path
        d="M9.75 9.30005C10.9926 9.30005 12 8.29268 12 7.05005C12 5.80742 10.9926 4.80005 9.75 4.80005C8.50737 4.80005 7.5 5.80742 7.5 7.05005C7.5 8.29268 8.50737 9.30005 9.75 9.30005Z"
        stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linejoin="round"></path>
    <path stroke={active ? '#2260F2' : '#383838'} stroke-width="1.5" stroke-linejoin="round"
        stroke-linecap="round"
        d="M3.45996 16.1994C3.61463 13.8543 5.56578 12 7.95006 12L11.5501 12C13.9312 12 15.8804 13.8494 16.0396 16.1901"></path>
</svg>
);

const ContractTypeIcon = ({ active }: { active: boolean }) => (
   <svg xmlns="http://www.w3.org/2000/svg"
    height="19.5" viewBox="0 0 19.5 19.5" fill="none">
    <path
        d="M6.75 0.75L1.75 0.75C1.19772 0.75 0.75 1.19772 0.75 1.75L0.75 6.75C0.75 7.3023 1.19772 7.75 1.75 7.75L6.75 7.75C7.3023 7.75 7.75 7.3023 7.75 6.75L7.75 1.75C7.75 1.19772 7.3023 0.75 6.75 0.75Z"
        stroke={active ? '#2260F2' : '#383838'}  stroke-width="1.5" stroke-linejoin="round"></path>
    <path
        d="M6.75 11.75L1.75 11.75C1.19772 11.75 0.75 12.1977 0.75 12.75L0.75 17.75C0.75 18.3023 1.19772 18.75 1.75 18.75L6.75 18.75C7.3023 18.75 7.75 18.3023 7.75 17.75L7.75 12.75C7.75 12.1977 7.3023 11.75 6.75 11.75Z"
        stroke={active ? '#2260F2' : '#383838'}  stroke-width="1.5" stroke-linejoin="round"></path>
    <path
        d="M17.75 0.75L12.75 0.75C12.1977 0.75 11.75 1.19772 11.75 1.75L11.75 6.75C11.75 7.3023 12.1977 7.75 12.75 7.75L17.75 7.75C18.3023 7.75 18.75 7.3023 18.75 6.75L18.75 1.75C18.75 1.19772 18.3023 0.75 17.75 0.75Z"
        stroke={active ? '#2260F2' : '#383838'}  stroke-width="1.5" stroke-linejoin="round"></path>
    <path
        d="M17.75 11.75L12.75 11.75C12.1977 11.75 11.75 12.1977 11.75 12.75L11.75 17.75C11.75 18.3023 12.1977 18.75 12.75 18.75L17.75 18.75C18.3023 18.75 18.75 18.3023 18.75 17.75L18.75 12.75C18.75 12.1977 18.3023 11.75 17.75 11.75Z"
        stroke={active ? '#2260F2' : '#383838'}  stroke-width="1.5" stroke-linejoin="round"></path>
</svg>
)

const CostIcon = ({ active }: { active: boolean }) => (
    <svg xmlns="http://www.w3.org/2000/svg" width="19.5" height="19.5" viewBox="0 0 19.5 19.5" fill="none">
        <path d="M2 17.25H17.75" stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinecap="round"/>
        <rect x="3.5" y="10" width="3" height="5.5" stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinejoin="round"/>
        <rect x="8.25" y="6" width="3" height="9.5" stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinejoin="round"/>
        <rect x="13" y="2.5" width="3" height="13" stroke={active ? '#2260F2' : '#383838'} strokeWidth="1.5" strokeLinejoin="round"/>
    </svg>
);

export default function SettingLayout({
    children,
}: {
    children: React.ReactNode
}) {
    const [user, setUser] = useState<User | null>(null);
    const [activeKey, setActiveKey] = useState<string>('model');
    const [loading, setLoading] = useState<boolean>(false);
    const [accessResolved, setAccessResolved] = useState<boolean>(false);
    const [loginVisible, setLoginVisible] = useState(false);
    const [registerVisible, setRegisterVisible] = useState(false);
    const router = useRouter();
    const pathname = usePathname();
    const isSystemAdmin = user?.system_role === 'admin' || user?.system_role === 'owner';
    const adminOnlyKeys = new Set(['contractType', 'risk']);
    const allowedUserPaths = ['/setting/about', '/setting/model', '/setting/cost'];
    const isAllowedSettingPath = isSystemAdmin || allowedUserPaths.some(p => pathname.includes(p));

    const getSelectedKey = useCallback(() => {
        if (pathname.includes('/setting/model')) return 'model';
        if (pathname.includes('/setting/cost')) return 'cost';
        if (pathname.includes('/setting/contractType')) return 'contractType';
        if (pathname.includes('/setting/risk')) return 'risk';
        if (pathname.includes('/setting/about')) return 'about';
        return 'model';
    }, [pathname]);

    const handleLoginClick = useCallback(() => {
        setLoginVisible(true);
    }, []);

    // 注册登录回调，使 403 模态框可以触发登录模态框
    useEffect(() => {
        const unregister = authDatedHandler.registerLoginCallback(() => {
            handleLoginClick();
        });
        return () => {
            unregister();
        };
    }, [handleLoginClick]);

    useEffect(() => {
        const checkLoginStatus = async () => {
            const token = localStorage.getItem('access_token');
            if (!token) {
                setAccessResolved(true);
                return;
            }

            try {
                // Force refresh so an old sessionStorage entry created before
                // system_role existed cannot expose stale privileged UI.
                const userInfo = await getUserInfo(true);
                setUser(userInfo);
            } catch {
                localStorage.removeItem('access_token');
                localStorage.removeItem('refresh_token');
                localStorage.removeItem('token_type');
                setUser(null);
            } finally {
                setAccessResolved(true);
            }
        };

        checkLoginStatus();
    }, []);

    useEffect(() => {
        setActiveKey(getSelectedKey());
        // Redirect non-admin users away from admin-only pages
        if (accessResolved && !isSystemAdmin && adminOnlyKeys.has(getSelectedKey())) {
            router.replace('/setting/about');
        }
    }, [accessResolved, getSelectedKey, isSystemAdmin, pathname, router]);

    const handleLoginSuccess = async (token: string) => {
        try {
            const userInfo = await getUserInfo();
            setUser(userInfo);
            if (token) {
                saveTokenInfo({ access_token: token });
            }
        } catch (error) {
            console.warn('获取用户信息失败:', error);
        }
        setLoginVisible(false);
        window.location.reload();
    };

    const handleRegisterSuccess = () => {
        setRegisterVisible(false);
        setLoginVisible(true);
    };

    const handleSwitchToRegister = () => {
        setLoginVisible(false);
        setRegisterVisible(true);
    };

    const handleSwitchToLogin = () => {
        setRegisterVisible(false);
        setLoginVisible(true);
    };

    const handleMenuClick = (key: string, path: string) => {
        setLoading(true);
        setActiveKey(key);
        router.push(path);
        setLoading(false);
    };

    
const adminMenuItems = [
    {
        key: 'model',
        icon: <ModelIcon active={activeKey === 'model'} />,
        label: <span style={{ marginLeft: '1rem' }}>大模型配置</span>,
        onClick: () => handleMenuClick('model', '/setting/model'),
        className: '!h-[3rem] !leading-[3rem] !text-[0.95rem] !mt-0 !mb-[0.75rem] !rounded-none transition-all duration-200',
        style: {
            paddingLeft: '1.5rem',
            color: activeKey === 'model' ? '#2260F2' : '#383838'
        }
    },
    {
        key: 'cost',
        icon: <CostIcon active={activeKey === 'cost'} />,
        label: <span style={{ marginLeft: '1rem' }}>成本看板</span>,
        onClick: () => handleMenuClick('cost', '/setting/cost'),
        className: '!h-[3rem] !leading-[3rem] !text-[0.95rem] !mt-0 !mb-[0.75rem] !rounded-none transition-all duration-200',
        style: {
            paddingLeft: '1.5rem',
            color: activeKey === 'cost' ? '#2260F2' : '#383838'
        }
    },
    {
        key: 'contractType',
        icon: <ContractTypeIcon active={activeKey === 'contractType'} />,
        label: <span style={{ marginLeft: '1rem' }}>合同类型配置</span>,
        onClick: () => handleMenuClick('contractType', '/setting/contractType'),
        className: '!h-[3rem] !leading-[3rem] !text-[0.95rem] !mt-0 !mb-[0.75rem] !rounded-none transition-all duration-200',
        style: {
            paddingLeft: '1.5rem',
            color: activeKey === 'contractType' ? '#2260F2' : '#383838'
        }
    },
    {
        key: 'risk',
        icon: <RiskIcon active={activeKey === 'risk'} />,
        label: <span style={{ marginLeft: '1rem' }}>风险点配置</span>,
        onClick: () => handleMenuClick('risk', '/setting/risk'),
        className: '!h-[3rem] !leading-[3rem] !text-[0.95rem] !mt-0 !mb-[0.75rem] !rounded-none transition-all duration-200',
        style: {
            paddingLeft: '1.5rem',
            color: activeKey === 'risk' ? '#2260F2' : '#383838'
        }
    },
    {
        key: 'about',
        icon: <AboutIcon active={activeKey === 'about'} />,
        label: <span style={{ marginLeft: '1rem' }}>关于我们</span>,
        onClick: () => handleMenuClick('about', '/setting/about'),
        className: '!h-[3rem] !leading-[3rem] !text-[0.95rem] !mt-0 !mb-[0.75rem] !rounded-none transition-all duration-200',
        style: {
            paddingLeft: '1.5rem',
            color: activeKey === 'about' ? '#2260F2' : '#383838'
        }
    }
];
const menuItems = isSystemAdmin
    ? adminMenuItems
    : adminMenuItems.filter((item) => !adminOnlyKeys.has(item.key));

    return (
        <div className="flex flex-col h-screen overflow-hidden bg-[#f3f4f6]">
            <Topbar
                user={user}
                onLoginClick={() => { }}
                onLogoutClick={() => {
                    localStorage.removeItem("access_token");
                    localStorage.removeItem("refresh_token");
                    localStorage.removeItem("token_type");
                    setUser(null);
                }}
                activeTab={null}
            />
            <div className="flex flex-1 overflow-hidden bg-white">
                <div className="w-[20rem] px-[1.25rem]">
                    <Menu
                        selectedKeys={[activeKey]}
                        mode="inline"
                        items={menuItems}
                        className="border-none"
                        style={{
                            paddingTop: '1.5rem',
                            border: 'none'
                        }}
                    />
                </div>
                {/* 这里渲染子路由内容 */}
                <div className="flex-1 overflow-auto bg-[#f1f1f1] p-[1.75rem]">
                    <div className="bg-white h-[calc(100vh-64px-3.5rem)] overflow-y-auto !rounded">
                        {loading || !accessResolved || !isAllowedSettingPath ? (
                            <div className="flex items-center justify-center h-full">
                                <Spin size="large" />
                            </div>
                        ) : (
                            children
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
}
