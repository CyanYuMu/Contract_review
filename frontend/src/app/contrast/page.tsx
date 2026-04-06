'use client';

import React from 'react';
import PageLayout from '@/components/PageLayout';
import ContractContrastPanel from '@/components/contrast/ContractContrastPanel';

/**
 * 合同比对页面
 */
export default function ContrastPage() {
    return (
        <PageLayout activeTab="contrast">
                <div className="h-full mt-[53px] overflow-hidden">
                    <ContractContrastPanel />
                </div>
        </PageLayout>
    );
}
