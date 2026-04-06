'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

export default function SettingPage() {
    const router = useRouter();

    useEffect(() => {
        // 访问 /setting 时自动重定向到 /setting/model
        router.replace('/setting/model');
    }, [router]);

    return (
        <div className="flex items-center justify-center h-screen">
            <div className="text-center">
                <p className="text-lg text-gray-500">正在加载...</p>
            </div>
        </div>
    );
}
