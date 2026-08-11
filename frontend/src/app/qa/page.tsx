'use client';

import dynamic from 'next/dynamic';
import PageLayout from '@/components/PageLayout';

// 动态导入问答面板，禁用 SSR（使用了 fetch 流式与 window）
const QAPanel = dynamic(() => import('@/components/qa/QAPanel'), {
    ssr: false,
    loading: () => (
        <div className="flex items-center justify-center h-full">
            <p className="text-gray-500">加载中...</p>
        </div>
    ),
});

/**
 * 合同问答页面
 */
export default function QAPage() {
    return (
        <PageLayout activeTab="qa" showFooter={false}>
            <div className="flex-1 overflow-hidden p-4">
                <div className="h-full bg-white rounded shadow-sm border border-gray-200 overflow-hidden flex flex-col">
                    <QAPanel/>
                </div>
            </div>
        </PageLayout>
    );
}
