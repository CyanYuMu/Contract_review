'use client';

import dynamicImport from 'next/dynamic';

// CostDashboard 直接引入 echarts，echarts 不是 SSR 安全库，在 Next.js 15 + React 19
// 下服务端渲染会抛 "Cannot assign to read only property 'params'"。因此关闭 SSR，
// 仅在客户端渲染（与 /result 页加载 ContractResult 的做法一致）。
const CostDashboard = dynamicImport(() => import('@/components/setting/CostDashboard'), {
    ssr: false,
    loading: () => (
        <div className="flex items-center justify-center h-screen">
            <div className="text-center text-gray-500">正在加载成本看板...</div>
        </div>
    ),
});

/**
 * 成本看板页面（设置 - 大模型成本追踪）
 */
export default function CostPage() {
    return <CostDashboard/>;
}
