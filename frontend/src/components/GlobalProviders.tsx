'use client';

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
