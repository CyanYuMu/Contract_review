'use client';

import React from 'react';
import dynamic from 'next/dynamic';
import PageLayout from '@/components/PageLayout';

// 动态导入 Signboard 组件，禁用 SSR（因为它使用了 window 对象）
const Signboard = dynamic(() => import('@/components/signboard/Signboard'), {
    ssr: false,
    loading: () => (
        <div className="flex items-center justify-center h-full">
            <p className="text-gray-500">加载中...</p>
        </div>
    ),
});

/**
 * 数据看板页面
 */
export default function DataboardPage() {
    return (
        <PageLayout activeTab="databoard" showFooter={false}>
            <div className="flex-1 overflow-auto">
                <Signboard />
            </div>
        </PageLayout>
    );
}
