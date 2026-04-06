'use client';

import React from "react";
import ContractUploader from "../components/ContractUploader";
import PageLayout from "@/components/PageLayout";
import ReviewPanel from "@/components/ReviewPanel";

/**
 * 合同审阅页面（主页）
 */
export default function HomeClient() {
    return (
        <PageLayout activeTab="check">
            <div className="flex-1 flex overflow-hidden mt-[2.75rem] ml-[2.94rem]">
                <div className="flex-1 flex flex-col justify-center items-center p-4 bg-white rounded-[0.31rem] mr-[4.43rem] border-[0.06rem] border-[#e3e3e3]">
                    <div className="flex flex-col justify-center items-center mb-4">
                        <ContractUploader onUploadSuccess={() => {}} />
                    </div>
                </div>
                <ReviewPanel />
            </div>
        </PageLayout>
    );
}
