'use client';
import React, { useState, useEffect, useRef, useCallback } from 'react';
import { message, Spin } from 'antd';
import { LoadingOutlined } from '@ant-design/icons';
import type { Editor } from '@/lib/canvas-editor/editor';
import ContractContrastUploader, { type UploadedContrastFile } from '@/components/ContractContrastUploader';
import { useRouter } from 'next/navigation';
import { UploadStore } from '@/store/uploadStore';
import { ContrastuploadStore } from '@/store/ContrastuploadStore';
import { startComparisonTask } from '@/lib/api/contrastApi';
interface ContractContrastPanelProps {
  editor?: Editor | null;
}

type ContrastFileState = Partial<UploadedContrastFile>;

export default function ContractContrastPanel({ editor: _editor }: ContractContrastPanelProps) {
  void _editor;
  const [originalFileName, setOriginalFileName] = useState<string | null>(null);
  const [comparisonFileName, setComparisonFileName] = useState<string | null>(null);
  const [originalFileId, setOriginalFileId] = useState<number | null>(null);
  const [comparisonFileId, setComparisonFileId] = useState<number | null>(null);
  const [isComparing, setIsComparing] = useState<boolean>(false);
  const originalFileRef = useRef<ContrastFileState>({});
  const comparisonFileRef = useRef<ContrastFileState>({});
  const isComparingRef = useRef(false);
  const startedKeyRef = useRef<string | null>(null);
  const router = useRouter();
  const setData = UploadStore((state) => state.setData);
  const { originalFile: storeOriginalFile, comparisonFile: storeComparisonFile } = ContrastuploadStore();

  const syncOriginalFile = useCallback((file: ContrastFileState) => {
    originalFileRef.current = { ...originalFileRef.current, ...file };
    setOriginalFileName(originalFileRef.current.title || null);
    setOriginalFileId(originalFileRef.current.file_id || null);

    setData({
      original_file_url: originalFileRef.current.file_url,
      original_file_title: originalFileRef.current.title,
      original_file_type: originalFileRef.current.file_type,
      original_file_id: originalFileRef.current.file_id
    });
  }, [setData]);

  const syncComparisonFile = useCallback((file: ContrastFileState) => {
    comparisonFileRef.current = { ...comparisonFileRef.current, ...file };
    setComparisonFileName(comparisonFileRef.current.title || null);
    setComparisonFileId(comparisonFileRef.current.file_id || null);

    setData({
      comparison_file_url: comparisonFileRef.current.file_url,
      comparison_file_title: comparisonFileRef.current.title,
      comparison_file_type: comparisonFileRef.current.file_type,
      comparison_file_id: comparisonFileRef.current.file_id
    });
  }, [setData]);

  // 从store中同步持久化的数据到本地state
  useEffect(() => {
    // 同步标准文档数据
    if (storeOriginalFile.file_url) {
      syncOriginalFile(storeOriginalFile);
    }
    
    // 同步对比文档数据
    if (storeComparisonFile.file_url) {
      syncComparisonFile(storeComparisonFile);
    }
  }, [storeOriginalFile, storeComparisonFile, syncOriginalFile, syncComparisonFile]);
  
  // 处理原始文件上传成功
  const handleOriginalUploadSuccess = (file: UploadedContrastFile) => {
    console.log('原始文件上传成功:', file);
    syncOriginalFile(file);
    
    message.success('标准文档上传成功');
    
    startComparison(file.file_id, comparisonFileRef.current.file_id, file.title, comparisonFileRef.current.title);
  };

  // 处理对比文件上传成功
  const handleComparisonUploadSuccess = (file: UploadedContrastFile) => {
    console.log('对比文件上传成功:', file);
    syncComparisonFile(file);
    
    message.success('对比文档上传成功');
    
    startComparison(originalFileRef.current.file_id, file.file_id, originalFileRef.current.title, file.title);
  };

  // 启动对比任务
  const startComparison = useCallback(async (
    standardId = originalFileId,
    comparisonId = comparisonFileId,
    standardName = originalFileName || undefined,
    comparisonName = comparisonFileName || undefined
  ) => {
    console.log('开始启动对比任务:', { standardId, comparisonId });
    
    if (!standardId || !comparisonId) {
      console.log('文件信息不完整，无法启动对比任务');
      return;
    }
    const taskKey = `${standardId}:${comparisonId}`;
    if (isComparingRef.current || startedKeyRef.current === taskKey) {
      return;
    }
    isComparingRef.current = true;
    startedKeyRef.current = taskKey;
    
    setIsComparing(true);
    try {
      message.loading('正在启动对比任务...', 0);
      const comparisonTitle = `合同比对：${standardName || '标准文档'} vs ${comparisonName || '比对文档'}`;
      
      console.log('调用对比任务接口:', { standardId, comparisonId });
      const result = await startComparisonTask(standardId, comparisonId, comparisonTitle);
      
      console.log('对比任务接口调用成功:', result);
      
      // 保存对比结果到localStorage，供结果页面使用
      localStorage.setItem('comparison_result', JSON.stringify(result));
      
      message.destroy();
      message.success('对比任务启动成功');
      
      // 跳转到对比结果页面
      console.log('跳转到对比结果页面');
      router.push('/result');
    } catch (error) {
      startedKeyRef.current = null;
      message.destroy();
      const errorMessage = (error as Error).message || '对比任务启动失败，请重试';
      message.error(errorMessage);
      console.log('对比任务启动失败:', errorMessage);
    } finally {
      isComparingRef.current = false;
      setIsComparing(false);
    }
  }, [comparisonFileId, comparisonFileName, originalFileId, originalFileName, router]);

  // 监听两个文件都上传成功的情况
  useEffect(() => {
    if (originalFileId && comparisonFileId && originalFileName && comparisonFileName) {
      console.log('检测到两个文件都已上传，自动启动对比任务');
      startComparison(originalFileId, comparisonFileId, originalFileName, comparisonFileName);
    }
  }, [originalFileId, comparisonFileId, originalFileName, comparisonFileName, startComparison]);


return (
    <div className="relative flex flex-col md:flex-row justify-center items-start h-full w-full max-w-full overflow-hidden p-4">
      {/* 对比任务加载遮罩 */}
      {isComparing && (
        <div className="absolute inset-0 bg-white bg-opacity-70 flex flex-col items-center justify-center z-50">
          <Spin indicator={<LoadingOutlined style={{ fontSize: 48, color: '#2260f2' }} spin />} />
          <div className="mt-4 text-lg text-gray-700">正在进行合同比对，请稍候...</div>
        </div>
      )}
      {/* 标准文档区域 */}
      <div className="flex flex-col border-[1px] border-[rgba(227,227,227,1)] rounded-[5px] bg-[rgba(255,255,255,1)] p-6 h-[100%] opacity-[1] flex-1 max-w-[calc(45%-53px)] min-w-[300px]">
        
        <h3 style={{position: 'relative', width: '117px', height: '25px', opacity: '1', fontSize: '16px', fontWeight: '700', letterSpacing: '0px', lineHeight: '23.17px', color: 'rgba(0, 0, 0, 1)', textAlign: 'left', verticalAlign: 'middle', marginBottom: '8px'}}>
          <span style={{width: '5px', height: '25px', opacity: '1', background: 'rgba(34, 96, 242, 1)', position: 'absolute', left: '0', top: '50%', transform: 'translateY(-50%)'}}></span>
          <span style={{marginLeft: '12px',fontSize:'16px'}}>标准文档上传</span>
        </h3>
     
        <div className="flex-1 flex flex-col items-center justify-center">
          <ContractContrastUploader onUploadSuccess={handleOriginalUploadSuccess} label="标准文档" isOriginal={true} />
        </div>
      </div>
      <div className="w-px h-full hidden md:block" style={{border:' 2px dashed rgba(222, 222, 222, 1)', margin:'0 80px'}}></div>
      {/* 对比文档区域 */}
    
      <div className="flex flex-col border-[1px] border-[rgba(227,227,227,1)] rounded-[5px] bg-[rgba(255,255,255,1)] p-6 h-[100%] opacity-[1] flex-1 max-w-[calc(46%-53px)] min-w-[300px]">
     
        <h3 style={{position: 'relative', width: '117px', height: '25px', opacity: '1', fontSize: '16px', fontWeight: '700', letterSpacing: '0px', lineHeight: '23.17px', color: 'rgba(0, 0, 0, 1)', textAlign: 'left', verticalAlign: 'middle', marginBottom: '8px'}}>
          <span style={{width: '5px', height: '25px', opacity: '1', background: 'rgba(34, 96, 242, 1)', position: 'absolute', left: '0', top: '50%', transform: 'translateY(-50%)'}}></span>
          <span style={{marginLeft: '12px',fontSize:'16px'}}>比对文档上传</span>
        </h3>
       
        <div className="flex-1 flex flex-col items-center justify-center">
          <ContractContrastUploader onUploadSuccess={handleComparisonUploadSuccess} label="对比文档" isOriginal={false} />
        </div>
      </div>
   </div>
   
  );
}
