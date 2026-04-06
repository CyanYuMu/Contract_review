'use client';
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

// 定义单个文件的数据类型
interface FileData {
  title?: string;
  file_type?: string;
  file_url?: string;
  original_file_url?: string;
  original_file_title?: string;
  original_file_type?: string;
  comparison_file_url?: string;
  comparison_file_title?: string;
  comparison_file_type?: string;
  page_count?: number;
  original_file_page_count?: number;
  comparison_file_page_count?: number;
  file_id?: number;
  original_file_id?: number;
  comparison_file_id?: number;
}

// 定义完整的上传状态类型
interface ContrastUploadState {
  // 标准文档数据
  originalFile: FileData;
  // 比对文档数据
  comparisonFile: FileData;
  // 设置标准文档数据
  setOriginalFile: (fileData: Partial<FileData>) => void;
  // 设置比对文档数据
  setComparisonFile: (fileData: Partial<FileData>) => void;
  // 重置所有数据
  resetAll: () => void;
  // 重置标准文档数据
  resetOriginalFile: () => void;
  // 重置比对文档数据
  resetComparisonFile: () => void;
}

// 创建ContrastuploadStore
const STORAGE_KEY = 'contrast-upload-store';

export const ContrastuploadStore = create<ContrastUploadState>()(
  persist(
    (set) => ({
      // 初始状态
      originalFile: {},
      comparisonFile: {},
      
      // 设置标准文档数据
      setOriginalFile: (fileData) => set((state) => ({
          originalFile: {...state.originalFile, ...fileData}
      })),
      
      // 设置比对文档数据
      setComparisonFile: (fileData) => set((state) => ({
          comparisonFile: {...state.comparisonFile, ...fileData}
      })),
      
      // 重置所有数据
      resetAll: () => set({
          originalFile: {},
          comparisonFile: {}
      }),
      
      // 重置标准文档数据
      resetOriginalFile: () => set((state) => ({
          originalFile: {}
      })),
      
      // 重置比对文档数据
      resetComparisonFile: () => set((state) => ({
          comparisonFile: {}
      }))
    }),
    {
      name: STORAGE_KEY,
      // 选择性持久化，只保存文档URL等关键信息
      partialize: (state) => ({
        originalFile: {
          file_url: state.originalFile.file_url,
          original_file_url: state.originalFile.original_file_url,
          comparison_file_url: state.originalFile.comparison_file_url,
          title: state.originalFile.title,
          file_id: state.originalFile.file_id
        },
        comparisonFile: {
          file_url: state.comparisonFile.file_url,
          original_file_url: state.comparisonFile.original_file_url,
          comparison_file_url: state.comparisonFile.comparison_file_url,
          title: state.comparisonFile.title,
          file_id: state.comparisonFile.file_id
        }
      })
    }
  )
)