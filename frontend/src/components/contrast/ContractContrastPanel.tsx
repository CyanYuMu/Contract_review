'use client';
import React, { useState, useEffect } from 'react';
import { message, Spin } from 'antd';
import { LoadingOutlined } from '@ant-design/icons';
import type { Editor } from '@/lib/canvas-editor/editor';
import ContractContrastUploader from '@/components/ContractContrastUploader';
import { useRouter } from 'next/navigation';
import { UploadStore } from '@/store/uploadStore';
import { ContrastuploadStore } from '@/store/ContrastuploadStore';
import { startComparisonTask } from '@/lib/api/contrastApi';
import { createSession } from '@/lib/api/createSession';
interface ContractContrastPanelProps {
  editor?: Editor | null;
}

export default function ContractContrastPanel({ editor }: ContractContrastPanelProps) {
  const [originalFile, setOriginalFile] = useState<string | null>(null);
  const [comparisonFile, setComparisonFile] = useState<string | null>(null);
  const [originalFileName, setOriginalFileName] = useState<string | null>(null);
  const [comparisonFileName, setComparisonFileName] = useState<string | null>(null);
  const [originalFileId, setOriginalFileId] = useState<number | null>(null);
  const [comparisonFileId, setComparisonFileId] = useState<number | null>(null);
  const [isComparing, setIsComparing] = useState<boolean>(false);
  const router = useRouter();
  const setData = UploadStore((state) => state.setData);
  const { originalFile: storeOriginalFile, comparisonFile: storeComparisonFile } = ContrastuploadStore();

  // 从store中同步持久化的数据到本地state
  useEffect(() => {
    // 同步标准文档数据
    if (storeOriginalFile.file_url) {
      setOriginalFile(storeOriginalFile.file_url);
      setOriginalFileName(storeOriginalFile.title || null);
      setOriginalFileId(storeOriginalFile.file_id || null);
      
      // 同时更新UploadStore，确保与合同审阅功能保持一致
      setData({
        original_file_url: storeOriginalFile.file_url,
        original_file_title: storeOriginalFile.title || '',
        original_file_type: storeOriginalFile.file_type || '' ,
        original_file_id: storeOriginalFile.file_id
      });
    }
    
    // 同步对比文档数据
    if (storeComparisonFile.file_url) {
      setComparisonFile(storeComparisonFile.file_url);
      setComparisonFileName(storeComparisonFile.title || null);
      setComparisonFileId(storeComparisonFile.file_id || null);
      
      // 同时更新UploadStore，确保与合同审阅功能保持一致
      setData({
        comparison_file_url: storeComparisonFile.file_url,
        comparison_file_title: storeComparisonFile.title,
        comparison_file_type: storeComparisonFile.file_type,
        comparison_file_id: storeComparisonFile.file_id
      });
    }
  }, [storeOriginalFile, storeComparisonFile, setData]);
  
  type ComparisonItem =
  | { id: string; type: 'added' | 'deleted'; content: string; position: string }
  | { id: string; type: 'modified'; original: string; modified: string; position: string };

  const handleGotoPosition = (position: string) => {
    // 使用editor实例定位到指定位置
    if (editor) {
      // 这里需要根据实际的editor API实现定位逻辑
      console.log('跳转到', position);
    }
  };

  // 处理原始文件上传成功
  const handleOriginalUploadSuccess = () => {
    const fileUrl = localStorage.getItem('original_file_url');
    const fileName = localStorage.getItem('original_file_title');
    const fileType = localStorage.getItem('original_file_type');
    const fileId = localStorage.getItem('original_file_id');
    
    console.log('原始文件上传成功:', { fileUrl, fileName, fileType, fileId });
    
    setOriginalFile(fileUrl || null);
    setOriginalFileName(fileName || null);
    setOriginalFileId(fileId ? Number(fileId) : null);
    
    // 保存到全局状态
    setData({
      original_file_url: fileUrl ?? undefined,
      original_file_title: fileName ?? undefined,
      original_file_type: fileType ?? undefined,
      original_file_id: fileId ? Number(fileId) : undefined
    });
    
    message.success('标准文档上传成功');
    
    // 如果两个文件都已上传，启动对比任务
    console.log('检查是否启动对比任务:', { comparisonFile, comparisonFileId });
    if (comparisonFile && comparisonFileId) {
      console.log('启动对比任务');
      startComparison();
    }
  };

  // 处理对比文件上传成功
  const handleComparisonUploadSuccess = () => {
    const fileUrl = localStorage.getItem('comparison_file_url');
    const fileName = localStorage.getItem('comparison_file_title');
    const fileType = localStorage.getItem('comparison_file_type');
    const fileId = localStorage.getItem('comparison_file_id');
    
    console.log('对比文件上传成功:', { fileUrl, fileName, fileType, fileId });
    
    setComparisonFile(fileUrl || null);
    setComparisonFileName(fileName || null);
    setComparisonFileId(fileId ? Number(fileId) : null);
    
    // 保存到全局状态
    setData({
      comparison_file_url: fileUrl ?? undefined,
      comparison_file_title: fileName ?? undefined,
      comparison_file_type: fileType ?? undefined,
      comparison_file_id: fileId ? Number(fileId) : undefined
    });
    
    message.success('对比文档上传成功');
    
    // 如果两个文件都已上传，启动对比任务
    console.log('检查是否启动对比任务:', { originalFile, originalFileId });
    if (originalFile && originalFileId) {
      console.log('启动对比任务');
      startComparison();
    }
  };

  // 监听两个文件都上传成功的情况
  useEffect(() => {
    if (originalFileId && comparisonFileId && originalFileName && comparisonFileName) {
      console.log('检测到两个文件都已上传，自动启动对比任务');
      startComparison();
    }
  }, [originalFileId, comparisonFileId, originalFileName, comparisonFileName]);

  // 启动对比任务
  const startComparison = async () => {
    console.log('开始启动对比任务:', { originalFileId, comparisonFileId });
    
    if (!originalFileId || !comparisonFileId) {
      message.error('文件信息不完整，无法启动对比任务');
      console.log('文件信息不完整，无法启动对比任务');
      return;
    }
    
    setIsComparing(true);
    try {
      message.loading('正在创建会话...', 0);
      
      // 创建会话
      const sessionData = {
        title: `合同比对：${originalFileName} vs ${comparisonFileName}`,
        session_type: 'compare'
      };
      
      console.log('创建会话:', sessionData);
      const sessionResult = await createSession(sessionData);
      const sessionId = sessionResult.session_id;
      
      console.log('会话创建成功:', sessionId);
      
    
      
      message.destroy();
      message.loading('正在启动对比任务...', 0);
      
      // 调用对比任务接口，并传递sessionId
      console.log('调用对比任务接口:', { originalFileId, comparisonFileId, sessionId });
      const result = await startComparisonTask(originalFileId, comparisonFileId, `合同比对：${originalFileName} vs ${comparisonFileName}`, sessionId);
      
      console.log('对比任务接口调用成功:', result);
      
      // 保存对比结果到localStorage，供结果页面使用
      localStorage.setItem('comparison_result', JSON.stringify(result));
      
      message.destroy();
      message.success('对比任务启动成功');
      
      // 跳转到对比结果页面
      console.log('跳转到对比结果页面');
      router.push('/result');
    } catch (error) {
      message.destroy();
      const errorMessage = (error as Error).message || '对比任务启动失败，请重试';
      message.error(errorMessage);
      console.log('对比任务启动失败:', errorMessage);
    } finally {
      setIsComparing(false);
    }
  };


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