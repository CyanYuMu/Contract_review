'use client';

import dynamicImport from 'next/dynamic';

const ContractResult = dynamicImport(() => import('../../components/ContractResult'), {
    ssr: false,
    loading: () => (
        <div className="flex items-center justify-center h-screen">
            <div className="text-center">
                <p className="text-lg text-gray-500">正在加载比对结果...</p>
            </div>
        </div>
    ),
});

export const dynamic = 'force-dynamic';

export default function ResultPage() {
    return <ContractResult />;
}