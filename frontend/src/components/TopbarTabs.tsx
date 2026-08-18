'use client';

import React, {useState} from 'react';
import {Button, Popover} from 'antd';
import Image from 'next/image';
import type {User} from "@/lib/Interface";
import {assets} from "@/assets/assets";
import {useRouter} from "next/navigation";

export type TabType = 'check' | 'qa' | 'contrast' | 'history';

// Tab 对应的路由路径
const tabRoutes: Record<TabType, string> = {
    check: '/',
    qa: '/qa',
    contrast: '/contrast',
    history: '/history',
};

type TopbarTabsProps = {
    user?: User | null;
    onLoginClick?: () => void;
    onLogoutClick?: () => void;
    activeTab?: TabType | null;
    onTabClick?: (tab: TabType) => void;
};

export default function TopbarTabs({
                                       user,
                                       onLoginClick,
                                       onLogoutClick,
                                       activeTab: externalActiveTab,
                                       onTabClick,
                                   }: TopbarTabsProps) {
    const [popoverOpen, setPopoverOpen] = useState(false);
    const activeTab = externalActiveTab ?? null;
    const router = useRouter();

    // 点击标签时进行路由跳转
    const handleTabClick = (tab: TabType) => {
        if (onTabClick) {
            onTabClick(tab);
            return;
        }

        // "合同审阅"始终回到首页（重新上传入口）；审阅中的工作区通过"智审记录"进入，
        // 避免 review_workspace_active 残留导致点击"合同审阅"仍停留在带合同的 /review。
        if (tab === "check") {
            if (typeof window !== "undefined") {
                window.localStorage.removeItem("review_workspace_active");
            }
            router.push(tabRoutes[tab]);
            return;
        }

        if (
            tab === "contrast" &&
            typeof window !== "undefined" &&
            window.localStorage.getItem("contrast_workspace_active") === "1"
        ) {
            router.push("/result");
            return;
        }

        router.push(tabRoutes[tab]);
    };

    const userLabel = user ? user.username : '登录';

    const tabs = [
        {key: 'check' as TabType, label: '合同审阅'},
        {key: 'qa' as TabType, label: '合同问答'},
        {key: 'contrast' as TabType, label: '合同比对'},
        {key: 'history' as TabType, label: '智审记录'},
    ];

    const popoverContent = (
        <div className='flex flex-col gap-2 min-w-[120px]'>
            {user ? (
                <>
                    <div className='text-sm font-medium text-gray-700 pb-2 '>
                        用户名：{user.username}
                    </div>
                    <Button
                        type='text'
                        onClick={() => router.push('/setting')}
                    >设置
                    </Button>
                    <Button
                        type='text'
                        danger
                        block
                        onClick={() => {
                            setPopoverOpen(false);
                            if (onLogoutClick) onLogoutClick();
                        }}
                    >
                        登出
                    </Button>
                </>
            ) : (
                <Button
                    type='primary'
                    block
                    onClick={() => {
                        setPopoverOpen(false);
                        if (onLoginClick) onLoginClick();
                    }}
                >
                    登录
                </Button>
            )}
        </div>
    );

    const activeIndex = activeTab ? tabs.findIndex(tab => tab.key === activeTab) : -1;

    return (
        <div className='flex items-end gap-4' style={{marginBottom: 0, paddingBottom: 0}}>
            <div
                className='flex items-center gap-1 bg-[#2260f2]'
                style={{
                    marginBottom: 0,
                    padding: '0.25rem 0.25rem 0 0.25rem',
                    borderTopLeftRadius: '0.5rem',
                    borderTopRightRadius: '0.5rem',
                    borderBottomLeftRadius: 0,
                    borderBottomRightRadius: 0,
                    position: 'relative'
                }}
            >
                {tabs.map((tab) => (
                    <button
                        key={tab.key}
                        onClick={() => handleTabClick(tab.key)}
                        className={`px-4 py-2 transition-all text-[1.13rem] cursor-pointer tracking-[0.13rem] ${
                            activeTab === tab.key
                                ? 'bg-gray-100 text-[#2260f2] font-bold'
                                : 'text-white hover:bg-[#2260f2]/80 font-normal'
                        }`}
                        style={{
                            width: '6.75rem',
                            marginBottom: 0,
                            whiteSpace: 'nowrap',
                            borderBottomLeftRadius: 0,
                            borderBottomRightRadius: 0,
                            padding: "0.5rem 0.88rem 0.5rem 0.88rem"
                        }}
                        type="button"
                    >
                        {tab.label}
                    </button>
                ))}

                {activeIndex >= 0 && (
                    <div
                        className='absolute bottom-0 h-1 bg-[#2260f2] transition-all duration-300 ease-in-out'
                        style={{
                            width: '3.5rem',
                            left: `${1.875 + activeIndex * 7}rem`,
                        }}
                    />
                )}
            </div>

            <Popover
                content={popoverContent}
                trigger='click'
                placement='bottomRight'
                open={popoverOpen}
                onOpenChange={setPopoverOpen}
                zIndex={10001}
            >
                <button
                    className='flex items-center justify-center hover:opacity-90 rounded-full p-2 cursor-pointer mr-[1.25rem]'
                    aria-label={userLabel}
                    title={userLabel}
                    style={{color: 'white', fontSize: '2rem'}}
                    type="button"
                >
                    <Image src={assets.UserIcon} alt="User" className=' w-[2rem] h-[2rem]'/>
                </button>
            </Popover>
        </div>
    );
}
