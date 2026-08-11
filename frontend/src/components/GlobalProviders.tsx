'use client';

// 必须在引入/使用 antd 之前加载 React 19 兼容补丁：它通过 unstableSetRender
// 把 antd 的命令式渲染（message/notification/Modal.confirm）切到 createRoot，
// 否则会触发 "[antd: compatible] antd v5 support React is 16 ~ 18" 警告。
// 放在 'use client' 根包装组件里，确保在客户端首屏前执行，早于任何 message.* 调用。
import '@ant-design/v5-patch-for-react-19';

import React from 'react';
import AuthDatedModal from '@/components/auth/AuthDatedModal';

/**
 * 全局 Provider 组件
 * 包含需要在客户端渲染的全局组件，如 AuthDatedModal
 */
export default function GlobalProviders({ children }: { children: React.ReactNode }) {
    return (
        <>
            {children}
            <AuthDatedModal />
        </>
    );
}
