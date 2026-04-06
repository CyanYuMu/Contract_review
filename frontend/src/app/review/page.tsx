'use client';

import dynamicImport from 'next/dynamic';
import './page.css';

const ReviewPageContent = dynamicImport(() => import('./ReviewPageContent'), {
    ssr: false,
    loading: () => (
        <div className="flex items-center justify-center h-screen">
            <div className="text-center">
                <p className="text-lg text-gray-500">正在加载...</p>
            </div>
        </div>
    ),
});

export const dynamic = 'force-dynamic';

export default function Page() {
    return <ReviewPageContent />;
}
