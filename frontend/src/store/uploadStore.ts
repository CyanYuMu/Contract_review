'use client';
import {create} from 'zustand'

type UploadData = {
    file_id?: number;
    title?: string;
    file_type?: string;
    file_url?: string;
    contract_type_id?: number;
    party_a?: string;
    party_b?: string;
    // 比对文档相关字段
    original_file_id?: number;
    original_file_url?: string;
    original_file_title?: string;
    original_file_type?: string;
    comparison_file_id?: number;
    comparison_file_url?: string;
    comparison_file_title?: string;
    comparison_file_type?: string;
};

type UploadState = {
    data: UploadData;
    setData: (newData: UploadData | null) => void;
    resetData: () => void;
}
export const UploadStore = create<UploadState>((set) => ({
    data: {},
    setData: (data) => set({data: data || {}}),
    resetData: () => set({data: {}})
}))
