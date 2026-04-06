'use client';

import React, { useState } from 'react';
import PageLayout from '@/components/PageLayout';
import ReviewHistory from '@/components/list/ReviewHistory';
import ContrastHistory from '@/components/list/ContrastHistory';

/**
 * 智审记录页面
 */
export default function HistoryPage() {
    const [historyType, setHistoryType] = useState<'review' | 'contrast'>('review');

    return (
        <PageLayout activeTab="history">
            <div className="flex-1 overflow-auto">
                <div className="h-full bg-[#f3f4f6] rounded-[0.31rem] border border-[#e3e3e3] shadow-sm p-6 overflow-auto">
                    {historyType === 'review' ? (
                        <ReviewHistory
                            type="Review"
                            onTypeChange={(type) =>
                                setHistoryType(type === 'Review' ? 'review' : 'contrast')
                            }
                        />
                    ) : (
                        <ContrastHistory
                            type="Contrast"
                            onTypeChange={(type) =>
                                setHistoryType(type === 'Contrast' ? 'contrast' : 'review')
                            }
                        />
                    )}
                </div>
            </div>
        </PageLayout>
    );
}
