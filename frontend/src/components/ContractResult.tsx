'use client';
import React, { useState, useEffect, useRef, useCallback } from 'react';
import { Button } from 'antd';
import { ContrastuploadStore } from '@/store/ContrastuploadStore';
import Topbar from '@/components/Topbar';
import type { TabType } from '@/components/TopbarTabs';
import { useRouter } from 'next/navigation';
import { resolveFileUrl } from '@/utils/url';
import { startComparisonTask } from '@/lib/api/contrastApi';
import { getUserInfo } from '@/lib/api/user';
import { clearTokenInfo } from '@/utils/client';
import { logout } from '@/lib/api/logout';
import LoginModal from '@/components/auth/LoginModal';
import RegisterModal from '@/components/auth/RegisterModal';
import type { User } from '@/lib/Interface';
import { authDatedHandler } from '@/utils/authDatedHandler';


type ComparisonItem = 
  | { id: string; type: 'added' | 'deleted'; content: string; position: string; isIgnored?: boolean }
  | { id: string; type: 'modified'; original: string; modified: string; position: string; char_diff?: any[]; isIgnored?: boolean }; 

export default function ContractResult() {
  const router = useRouter();
  const [activeTab, setActiveTab] = useState<TabType>('contrast');
  const [comparisonResults, setComparisonResults] = useState<ComparisonItem[]>([]);
  const [selectedResult, setSelectedResult] = useState<ComparisonItem | null>(null);
  const [activeMenu, setActiveMenu] = useState<string>('全部');
  const [syncScroll, setSyncScroll] = useState(false);
  const [originalZoomLevel, setOriginalZoomLevel] = useState(100); // 默认
  const [comparisonZoomLevel, setComparisonZoomLevel] = useState(100); // 默认
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeMenuPosition, setActiveMenuPosition] = useState(0);
  const [activeMenuWidth, setActiveMenuWidth] = useState(20);
  const [originalScrollPercentage, setOriginalScrollPercentage] = useState(0);
  const [comparisonScrollPercentage, setComparisonScrollPercentage] = useState(0);
  const isScrollingRef = useRef(false);
  
  // 用户状态和登录相关
  const [user, setUser] = useState<User | null>(null);
  const [loginVisible, setLoginVisible] = useState(false);
  const [registerVisible, setRegisterVisible] = useState(false);

  // 获取用户信息
  useEffect(() => {
    const checkLoginStatus = async () => {
      const token = localStorage.getItem('access_token');
      if (token) {
        try {
          const userInfo = await getUserInfo();
          setUser(userInfo);
        } catch {
          localStorage.removeItem('access_token');
          setUser(null);
        }
      }
    };
    checkLoginStatus();
  }, []);

  const handleLoginClick = useCallback(() => {
    setLoginVisible(true);
  }, []);

  // 注册登录回调，使 403/401 模态框可以触发登录模态框
  useEffect(() => {
    const unregister = authDatedHandler.registerLoginCallback(() => {
      handleLoginClick();
    });
    return () => {
      unregister();
    };
  }, [handleLoginClick]);

  const handleLoginSuccess = async (token: string) => {
    try {
      const userInfo = await getUserInfo(true);
      setUser(userInfo);
      if (token) localStorage.setItem('access_token', token);
    } catch (error) {
      console.warn('获取用户信息失败:', error);
    }
    setLoginVisible(false);
    window.location.reload();
  };

  const handleLogout = async () => {
    try {
      await logout();
    } catch (error) {
      console.warn('登出失败:', error);
    } finally {
      clearTokenInfo();
      setUser(null);
    }
  };

  const handleSwitchToRegister = () => {
    setLoginVisible(false);
    setRegisterVisible(true);
  };

  const handleSwitchToLogin = () => {
    setRegisterVisible(false);
    setLoginVisible(true);
  };

  const handleRegisterSuccess = () => {
    setRegisterVisible(false);
    setLoginVisible(true);
  };
  const buildComparisonResults = (diffs: any[]): ComparisonItem[] => {
    const items = diffs.map((diff: any, index: number): ComparisonItem | null => {
      const position = `第${diff.std_index || diff.cmp_index || 1}段`;
      if (diff.operation === 'add' || diff.operation === 'insert') {
        return {
          id: `added-${index}`,
          type: 'added' as const,
          content: `${diff.comparison_text || '新增内容'}`,
          position
        };
      }
      if (diff.operation === 'delete' || diff.operation === 'remove') {
        return {
          id: `deleted-${index}`,
          type: 'deleted' as const,
          content: `标准合同：${diff.standard_text || '删除内容'}`,
          position
        };
      }
      if (diff.operation === 'replace' || diff.operation === 'update') {
        return {
          id: `modified-${index}`,
          type: 'modified' as const,
          original: `标准合同：${diff.standard_text || '原始内容'}`,
          modified: `${diff.comparison_text || '修改内容'}`,
          position,
          char_diff: diff.char_diff || []
        };
      }
      return null;
    });
    return items.filter((item): item is ComparisonItem => item !== null);
  }; 
  
  // 添加按钮引用
  const allButtonRef = useRef<HTMLButtonElement>(null);
  const addedButtonRef = useRef<HTMLButtonElement>(null);
  const deletedButtonRef = useRef<HTMLButtonElement>(null);
  const modifiedButtonRef = useRef<HTMLButtonElement>(null);
  const ignoredButtonRef = useRef<HTMLButtonElement>(null);
  
  // 处理忽略按钮点击
  const handleIgnore = (e: React.MouseEvent, itemId: string) => {
    e.stopPropagation(); // 阻止事件冒泡，避免触发卡片点击
    setComparisonResults(prevResults => 
      prevResults.map(item => 
        item.id === itemId ? { ...item, isIgnored: true } : item
      )
    );
  };

  // 当comparisonResults变化时，重新应用高亮效果
  useEffect(() => {
    // 获取iframe文档并重新应用高亮
    const originalIframe = originalIframeRef.current;
    const comparisonIframe = comparisonIframeRef.current;
    
    if (originalIframe?.contentDocument && comparisonIframe?.contentDocument) {
      applyDiffHighlighting(originalIframe.contentDocument, true);
      applyDiffHighlighting(comparisonIframe.contentDocument, false);
    }
  }, [comparisonResults]);

  
  // 添加iframe引用状态
  const originalIframeRef = useRef<HTMLIFrameElement>(null);
  const comparisonIframeRef = useRef<HTMLIFrameElement>(null);
  const TypeColorMap={
    added:'rgba(34, 96, 242, 1)',
    deleted:'rgba(212, 48, 48, 1)',
    modified:'rgba(255, 195, 0, 1)',
    ignored:'rgba(56, 56, 56, 1)'
  };
  
  // 统计不同类型的结果数量
  const resultStats = {
    total: comparisonResults.length,
    added: comparisonResults.filter(result => result.type === 'added').length,
    deleted: comparisonResults.filter(result => result.type === 'deleted').length,
    modified: comparisonResults.filter(result => result.type === 'modified').length,
    ignored: comparisonResults.filter(result => (result as { isIgnored?: boolean }).isIgnored).length
  };
  
  // 添加菜单容器引用
  const menuContainerRef = useRef<HTMLDivElement>(null);
  
  // 计算蓝色滑动短线的位置
  useEffect(() => {
    const updateIndicatorPosition = () => {
      let buttonRef;
    switch (activeMenu) {
      case '全部':
        buttonRef = allButtonRef.current;
        break;
      case '增加':
        buttonRef = addedButtonRef.current;
        break;
      case '删除':
        buttonRef = deletedButtonRef.current;
        break;
      case '修改':
        buttonRef = modifiedButtonRef.current;
        break;
      case '忽略':
        buttonRef = ignoredButtonRef.current;
        break;
      default:
        buttonRef = allButtonRef.current;
    }
      
      if (buttonRef && menuContainerRef.current) {
    
        const buttonRect = buttonRef.getBoundingClientRect();
        const containerRect = menuContainerRef.current.getBoundingClientRect();
        
     
        const position = buttonRect.left - containerRect.left + menuContainerRef.current.scrollLeft + buttonRect.width/2 - 10; // 减去蓝色条宽度的一半(20/2=10)以实现居中
        const width = 20; 
        
        setActiveMenuPosition(position);
        setActiveMenuWidth(width);
      }
    };
    
    // 初始加载时更新位置
    updateIndicatorPosition();
    
    // 监听窗口大小变化，重新计算位置
    window.addEventListener('resize', updateIndicatorPosition);
    
    // 当activeMenu变化时更新位置
    const timer = setTimeout(updateIndicatorPosition, 100);
    
    // 添加滚动事件监听，确保滚动时位置正确
    const menuContainer = menuContainerRef.current;
    if (menuContainer) {
      menuContainer.addEventListener('scroll', updateIndicatorPosition);
    }
    
    return () => {
      window.removeEventListener('resize', updateIndicatorPosition);
      clearTimeout(timer);
      if (menuContainer) {
        menuContainer.removeEventListener('scroll', updateIndicatorPosition);
      }
    };
  }, [activeMenu, resultStats]);
  
  // 获取全局状态中的文档数据
  const { originalFile, comparisonFile } = ContrastuploadStore();
  
  // 用于渲染文档的DOM节点引用
  const originalDocRef = useRef<HTMLDivElement>(null);
  const comparisonDocRef = useRef<HTMLDivElement>(null);
  // 用于滚动同步的容器引用
  const originalScrollContainerRef = useRef<HTMLDivElement>(null);
  const comparisonScrollContainerRef = useRef<HTMLDivElement>(null);

  // 同步滚动处理函数（直接处理容器ref）
  const handleScrollSync = (sourceContainerRef: React.RefObject<HTMLDivElement>, targetContainerRef: React.RefObject<HTMLDivElement>) => {
    if (!syncScroll || isScrollingRef.current) return;

    const sourceContainer = sourceContainerRef.current;
    const targetContainer = targetContainerRef.current;

    if (!sourceContainer || !targetContainer) return;

    isScrollingRef.current = true;

    // 使用requestAnimationFrame确保滚动计算在重绘前执行
    requestAnimationFrame(() => {
      try {
        const sourceScrollTop = sourceContainer.scrollTop;
        const sourceScrollHeight = sourceContainer.scrollHeight;
        const sourceClientHeight = sourceContainer.clientHeight;
        // 避免除以0（内容高度小于可视高度时无需滚动）
        const sourceScrollableHeight = Math.max(sourceScrollHeight - sourceClientHeight, 1);
        const sourceScrollPercentage = sourceScrollTop / sourceScrollableHeight;

        // 更新滚动百分比状态
        if (sourceContainerRef === originalScrollContainerRef) {
          setOriginalScrollPercentage(sourceScrollPercentage * 100);
        } else if (sourceContainerRef === comparisonScrollContainerRef) {
          setComparisonScrollPercentage(sourceScrollPercentage * 100);
        }

        const targetScrollHeight = targetContainer.scrollHeight;
        const targetClientHeight = targetContainer.clientHeight;
        const targetScrollableHeight = Math.max(targetScrollHeight - targetClientHeight, 1);

      
        const targetScrollTop = Math.min(
          Math.max(sourceScrollPercentage * targetScrollableHeight, 0),
          targetScrollableHeight
        );

        targetContainer.scrollTop = targetScrollTop;

        // 同时更新目标容器的滚动百分比状态
        if (targetContainerRef === originalScrollContainerRef) {
          setOriginalScrollPercentage(sourceScrollPercentage * 100);
        } else if (targetContainerRef === comparisonScrollContainerRef) {
          setComparisonScrollPercentage(sourceScrollPercentage * 100);
        }

      } catch (error) {
        console.error('同步滚动出错:', error);
      } finally {
        // 延迟释放锁，避免快速滚动时的抖动
        setTimeout(() => {
          isScrollingRef.current = false;
        }, 16); // 约一帧时间
      }
    });
  };


  // 标准文档缩放控制
  const handleOriginalZoomIn = () => {
    if (originalZoomLevel < 150) { 
      setOriginalZoomLevel(prev => prev + 10);
    }
  };

  const handleOriginalZoomOut = () => {
    if (originalZoomLevel > 30) { 
      setOriginalZoomLevel(prev => prev - 10);
    }
  };

  // 对比文档缩放控制
  const handleComparisonZoomIn = () => {
    if (comparisonZoomLevel < 150) { 
      setComparisonZoomLevel(prev => prev + 10);
    }
  };

  const handleComparisonZoomOut = () => {
    if (comparisonZoomLevel > 30) { 
      setComparisonZoomLevel(prev => prev - 10);
    }
  };

  // 应用缩放效果
  const applyZoom = (iframeRef: React.RefObject<HTMLIFrameElement>, zoomLevel: number) => {
    const iframe = iframeRef.current;
    if (!iframe) return;
    
    const iframeDoc = iframe.contentDocument;
    if (iframeDoc) {
      // 应用缩放样式到iframe内容
      iframeDoc.body.style.transform = `scale(${zoomLevel / 100})`;
      iframeDoc.body.style.transformOrigin = 'top left';
      iframeDoc.body.style.width = `${10000 / zoomLevel}%`; 
      iframeDoc.body.style.minHeight = `${10000 / zoomLevel}%`;
      
      // 缩放后强制重新计算高度并同步滚动
      setTimeout(() => {
        if (syncScroll) {
          alignDocumentPositions(); 
        }
      }, 100); 
    }
  };


  // 对齐两个文档的滚动位置
  const alignDocumentPositions = () => {
    const originalContainer = originalScrollContainerRef.current;
    const comparisonContainer = comparisonScrollContainerRef.current;

    if (!originalContainer || !comparisonContainer) return;

    try {
      // 获取标准文档滚动容器的滚动信息
      const originalScrollTop = originalContainer.scrollTop;
      const originalScrollHeight = originalContainer.scrollHeight;
      const originalClientHeight = originalContainer.clientHeight;
      
      // 计算滚动百分比
      const scrollPercentage = originalClientHeight < originalScrollHeight 
        ? originalScrollTop / (originalScrollHeight - originalClientHeight) 
        : 0;
      
  
      setOriginalScrollPercentage(scrollPercentage * 100);
      setComparisonScrollPercentage(scrollPercentage * 100);
      
      // 获取对比文档滚动容器的滚动信息
      const comparisonScrollHeight = comparisonContainer.scrollHeight;
      const comparisonClientHeight = comparisonContainer.clientHeight;
      
  
      const comparisonScrollTop = comparisonClientHeight < comparisonScrollHeight 
        ? scrollPercentage * (comparisonScrollHeight - comparisonClientHeight) 
        : 0;

  
      comparisonContainer.scrollTop = comparisonScrollTop;
      
      console.log('文档位置已对齐');
    } catch (error) {
      console.error('对齐文档位置出错:', error);
    }
  };


  useEffect(() => {
    const originalContainer = originalScrollContainerRef.current;
    const comparisonContainer = comparisonScrollContainerRef.current;

    if (!originalContainer || !comparisonContainer) return;

    const handleOriginalScroll = () => {
    if (originalScrollContainerRef.current) {
      const container = originalScrollContainerRef.current;
      const scrollPercentage = container.clientHeight < container.scrollHeight 
        ? (container.scrollTop / (container.scrollHeight - container.clientHeight)) * 100 
        : 0;
      setOriginalScrollPercentage(scrollPercentage);
      if (comparisonScrollContainerRef.current) {
        handleScrollSync(originalScrollContainerRef as React.RefObject<HTMLDivElement>, comparisonScrollContainerRef as React.RefObject<HTMLDivElement>);
      }
    }
  };
  const handleComparisonScroll = () => {
    if (comparisonScrollContainerRef.current) {
      const container = comparisonScrollContainerRef.current;
      const scrollPercentage = container.clientHeight < container.scrollHeight 
        ? (container.scrollTop / (container.scrollHeight - container.clientHeight)) * 100 
        : 0;
      setComparisonScrollPercentage(scrollPercentage);
      if (originalScrollContainerRef.current) {
        handleScrollSync(comparisonScrollContainerRef as React.RefObject<HTMLDivElement>, originalScrollContainerRef as React.RefObject<HTMLDivElement>);
      }
    }
  };

   
    if (syncScroll) {

      setTimeout(() => {
        alignDocumentPositions();
      }, 100);
    }

    // 监听外部滚动容器的滚动事件
    if (syncScroll) {
      originalContainer.addEventListener('scroll', handleOriginalScroll);
      comparisonContainer.addEventListener('scroll', handleComparisonScroll);
    }

    return () => {
      originalContainer.removeEventListener('scroll', handleOriginalScroll);
      comparisonContainer.removeEventListener('scroll', handleComparisonScroll);
    };
  }, [syncScroll]);

  // 当缩放级别变化时应用缩放效果
  useEffect(() => {
    if (originalIframeRef.current) applyZoom(originalIframeRef as React.RefObject<HTMLIFrameElement>, originalZoomLevel);
  }, [originalZoomLevel]);

  useEffect(() => {
    if (comparisonIframeRef.current) applyZoom(comparisonIframeRef as React.RefObject<HTMLIFrameElement>, comparisonZoomLevel);
  }, [comparisonZoomLevel]);
  
  
  // 监听差异结果变化，重新应用高亮
  useEffect(() => {
    // 确保文档已经渲染完成
    if (originalIframeRef.current && comparisonIframeRef.current) {
      setTimeout(() => {
        const originalIframe = originalIframeRef.current;
        const comparisonIframe = comparisonIframeRef.current;

        if (!originalIframe || !comparisonIframe) {
          console.warn('iframe 尚未挂载，跳过高亮');
          return;
        }

        const originalIframeDoc = originalIframe.contentDocument;
        const comparisonIframeDoc = comparisonIframe.contentDocument;

        if (!originalIframeDoc || !comparisonIframeDoc) {
          console.warn('无法访问 iframe 的 document，跳过高亮');
          return;
        }
        if (originalIframeDoc) {
          applyDiffHighlighting(originalIframeDoc, true);
        }
        
        if (comparisonIframeDoc) {
          applyDiffHighlighting(comparisonIframeDoc, false);
        }
      }, 1000);
    }
  }, [comparisonResults]);
  
  // 调用API获取对比结果
  useEffect(() => {
    const fetchComparisonResult = async () => {
      setLoading(true);
      setError(null);
      
      try {
        const historyRaw = localStorage.getItem('comparison_history_detail');
        if (historyRaw) {
          try {
            const historyData = JSON.parse(historyRaw);
            const historyPayload = historyData?.data ?? historyData;
            const historyDiffs = historyPayload?.diffs ?? historyPayload?.diff_list ?? [];
            if (Array.isArray(historyDiffs)) {
              setComparisonResults(buildComparisonResults(historyDiffs));
              setLoading(false);
              setError(null);
              localStorage.removeItem('comparison_history_detail');
              return;
            }
          } catch (e) {
            console.error('解析对比历史失败:', e);
          }
        }
        const latestRaw = localStorage.getItem('comparison_result');
        if (latestRaw) {
          try {
            const latestData = JSON.parse(latestRaw);
            const latestPayload = latestData?.data ?? latestData;
            const latestDiffs = latestPayload?.diffs ?? [];
            if (Array.isArray(latestDiffs)) {
              setComparisonResults(buildComparisonResults(latestDiffs));
              setLoading(false);
              setError(null);
              localStorage.removeItem('comparison_result');
              return;
            }
          } catch (e) {
            console.error('解析最新对比结果失败:', e);
          }
        }
        // 从状态中获取文档ID
        const standardFileId = originalFile?.file_id;
        const comparisonFileId = comparisonFile?.file_id;

        if (!standardFileId || !comparisonFileId) {
          setError('缺少文档ID，无法获取对比结果');
          return;
        }

        // 使用startComparisonTask函数获取对比结果
        const result = await startComparisonTask(
          standardFileId,
          comparisonFileId,
          `比对: ${originalFile?.title || '标准文档'} vs ${comparisonFile?.title || '对比文档'}`
        );

     
        const diffs = result.data?.diffs || [];
        setComparisonResults(buildComparisonResults(diffs));
      } catch (error: any) {
        console.error('获取对比结果失败:', error);
        setError(error.response?.data?.msg || '获取对比结果失败，请稍后重试');
      } finally {
        setLoading(false);
      }
    };

    fetchComparisonResult();
  }, [originalFile, comparisonFile]);
  
  // 应用差异高亮的函数
  const applyDiffHighlighting = (iframeDoc: Document, isOriginalDoc: boolean) => {
    // 确保有差异结果可以处理
    if (!comparisonResults || comparisonResults.length === 0) {
      console.log('没有差异结果可以应用高亮');
      return;
    }
    
    // 获取文档中的所有段落
    const paragraphs = iframeDoc.querySelectorAll('.docx-paragraph, p');
    if (paragraphs.length === 0) {
      console.log('没有找到文档段落');
      return;
    }
    
    console.log(`找到${paragraphs.length}个段落，开始应用高亮`);
    
    // 1. 清除所有现有的高亮样式
    paragraphs.forEach(paragraph => {
      const p = paragraph as HTMLElement;
   
      p.classList.remove('diff-added', 'diff-removed', 'diff-modified');
    
      const modifiedWords = p.querySelectorAll('.diff-modified-word');
      modifiedWords.forEach(word => {
       
        const textNode = document.createTextNode(word.textContent || '');
        word.parentNode?.replaceChild(textNode, word);
      });
    
      if (p.innerHTML !== p.textContent) {
        p.textContent = p.textContent || '';
      }

    });
    
    // 创建一个新的段落数组，用于处理可能的插入操作
    let updatedParagraphs = Array.from(paragraphs);
    
    // 遍历所有差异结果，跳过被忽略的结果
    comparisonResults.forEach((result) => {
      // 如果结果被忽略，跳过高亮处理
      if (result.isIgnored) {
        return;
      }
      
      // 解析位置信息，提取段落索引
      const positionMatch = result.position.match(/第(\d+)段/);
      if (!positionMatch) {
        console.log('无法解析位置信息:', result.position);
        return;
      }
      
      const paragraphIndex = parseInt(positionMatch[1]) - 1; // 转换为0-based索引
      
      // 检查段落索引是否有效
      if (paragraphIndex < 0 || paragraphIndex >= updatedParagraphs.length) {
        console.log('段落索引超出范围:', paragraphIndex);
        return;
      }
      
      const paragraph = updatedParagraphs[paragraphIndex] as HTMLElement;
      
      // 根据文档类型和差异类型应用不同的高亮
      if (isOriginalDoc) {
        // 原始文档（标准文档）
        // 标准文档不需要显示任何高亮样式和占位符
      } else {
        // 对比文档显示添加、删除或修改的内容
        if (result.type === 'added') {
          paragraph.classList.add('diff-added');
        } else if (result.type === 'deleted') {
          // 对比文档中删除的内容
          // 确保删除的内容在对比文档中显示并高亮
          if (paragraph) {
            paragraph.classList.add('diff-removed');
          } else {
            // 如果段落不存在，创建一个新的段落来显示删除的内容
            const deletedParagraph = document.createElement('p');
            deletedParagraph.className = 'docx-paragraph diff-removed';
            deletedParagraph.textContent = result.content.replace('删除条款：', '');
            
            // 将删除的内容插入到对比文档的相应位置
            if (paragraphIndex < updatedParagraphs.length) {
              updatedParagraphs[paragraphIndex].parentElement?.insertBefore(deletedParagraph, updatedParagraphs[paragraphIndex]);
              // 更新段落数组，确保后续的操作正确
              updatedParagraphs = Array.from(iframeDoc.querySelectorAll('.docx-paragraph, p'));
            }
          }
        } else if (result.type === 'modified') {
            // 对比文档中修改的内容
            // 添加段落级别的diff-modified类，确保修改的内容有高亮显示
            paragraph.classList.add('diff-modified');
            
            // 对于修改的内容，在对比文档中标记添加和删除的词语
            const originalText = result.original.replace('原条款：', '');
            const modifiedText = result.modified.replace('修改为：', '');
            
            // 使用后端返回的char_diff信息来精确高亮词语级差异
            let highlightedText = '';
            if (result.char_diff && result.char_diff.length > 0) {
              // 有精确的字符级差异信息，使用它来高亮
              let currentIndex = 0;
              const modifiedTextContent = modifiedText;
              
              // 对差异进行排序，确保从左到右处理
              const sortedDiff = [...result.char_diff].sort((a, b) => a.cmp_range[0] - b.cmp_range[0]);
              
              sortedDiff.forEach(diff => {
                // 添加差异前的正常文本
                highlightedText += modifiedTextContent.substring(currentIndex, diff.cmp_range[0]);
                
                // 添加差异文本，使用修改样式
                highlightedText += `<span class="diff-modified-word">${modifiedTextContent.substring(diff.cmp_range[0], diff.cmp_range[1])}</span>`;
                
                currentIndex = diff.cmp_range[1];
              });
              
              // 添加最后一个差异后的正常文本
              highlightedText += modifiedTextContent.substring(currentIndex);
            } else {
              // 没有字符级差异信息，使用简单的词语差异高亮
              highlightedText = highlightWordDifferences(originalText, modifiedText, false);
            }
            
            // 始终更新段落内容，确保修改的内容有高亮显示
            paragraph.innerHTML = highlightedText;
        }
      }
    });
    
    console.log('差异高亮应用完成');
  };
  
  // 词语级差异高亮函数
  const highlightWordDifferences = (original: string, modified: string, isOriginal: boolean) => {
    // 将文本拆分为词语数组
    const originalWords = original.split(/(\s+)/);
    const modifiedWords = modified.split(/(\s+)/);
    
    // 简单的词语差异检测（实际应用中可能需要更复杂的算法）
    if (originalWords.length !== modifiedWords.length) {
      // 如果词语数量不同，直接返回完整的高亮
      return isOriginal 
        ? `<span class="diff-removed">${original}</span>`
        : `<span class="diff-modified-word">${modified}</span>`;
    }
    
    // 对比每个词语
    const highlightedWords = modifiedWords.map((word, index) => {
      if (index < originalWords.length && word !== originalWords[index]) {
        if (isOriginal) {
          // 原始文档中被修改的词语：保留删除样式（红色）
          return `<span class="diff-removed">${originalWords[index]}</span>`;
        } else {
          // 对比文档中修改后的词语：使用修改样式（黄色）
          return `<span class="diff-modified-word">${word}</span>`;
        }
      }
      return isOriginal ? originalWords[index] : word;
    });
    
    return highlightedWords.join('');
  };
  
  // 渲染文档的函数（更新为支持iframe引用）
 const renderDocument = async (fileUrl: string | undefined, containerRef: React.RefObject<HTMLDivElement>, iframeRef: React.RefObject<HTMLIFrameElement>, isOriginal: boolean) => {
  const resolvedFileUrl = resolveFileUrl(fileUrl);

  if (!resolvedFileUrl || !containerRef.current) return;
  
  try {
    // 清空容器
    containerRef.current.innerHTML = '';
    
    // 创建一个iframe用于渲染文档
    const iframe = document.createElement('iframe');
    iframe.className = 'w-full border-none block';
    iframe.style.height = 'auto';
    containerRef.current.appendChild(iframe);
    
    // 将iframe关联到ref
    if (iframeRef) {
      iframeRef.current = iframe;
    }
    
    // 为文档容器添加明确的样式
    containerRef.current.style.position = 'relative';
    containerRef.current.style.overflow = 'hidden';
    containerRef.current.style.width = '100%';
    containerRef.current.style.height = 'auto';
    
    // 等待iframe加载完成
    await new Promise<void>((resolve) => {
      iframe.onload = () => resolve();
      iframe.srcdoc = `
        <!DOCTYPE html>
                <html>
                  <head>
                    <style>
                      * {
                        box-sizing: border-box;
                        margin: 0;
                        padding: 0;
                      }
                      body {
                        font-family: 'Microsoft YaHei', sans-serif;
                        line-height: 1.8;
                        font-size: 15px;
                        background: transparent;
                        height: 100%;
                        overflow: auto;
                      }
                      .docx-content-wrapper,
                      .docx-container {
                        background: rgba(222, 222, 222, 1) !important;
                        width: 100%;
                        height: auto;
                        min-height: 100%;
                        overflow-y: auto;
                        overflow-x: auto;
                        padding: 0px;
                      }
                      /* docx-preview 生成的包装器样式 */
                      .docx-wrapper {
                        background: rgba(222, 222, 222, 1) !important;
                        padding: 20px 0 !important;
                      }
                      .docx-wrapper > section.docx {
                        width: 100% !important;
                        max-width: 100% !important;
                        min-width: auto !important;
                        padding: 40px 30px !important;
                        margin: 20px auto !important;
                        box-sizing: border-box !important;
                        overflow-x: auto !important;
                      }
                      /* 表格自适应宽度 */
                      table {
                        max-width: 100% !important;
                        width: auto !important;
                        table-layout: auto !important;
                        overflow-x: auto !important;
                        display: block !important;
                        word-break: break-word !important;
                      }
                      /* 分页样式支持 */
                      .docx-page {
                        padding: 60px 40px;
                        background: white;
                        page-break-after: always;
                        position: relative;
                        max-width: 100% !important;
                        overflow-x: auto !important;
                      }
                      /* 页眉页脚样式支持 */
                      .docx-header {
                        position: absolute;
                        top: 0;
                        left: 0;
                        right: 0;
                        padding: 20px 40px;
                        background: white;
                        z-index: 10;
                      }
                      .docx-footer {
                        position: absolute;
                        bottom: 0;
                        left: 0;
                        right: 0;
                        padding: 20px 40px;
                        background: white;
                        z-index: 10;
                      }
                      /* 隐藏页码显示 */
                      footer, .docx-footer {
                        visibility: hidden !important;
                        height: 0 !important;
                        overflow: hidden !important;
                        padding: 0 !important;
                        margin: 0 !important;
                      }
                      .docx-paragraph, p {
                        margin: 12px 0;
                        padding: 0;
                        border-left: 4px solid transparent;
                        transition: all 0.3s ease;
                        page-break-inside: avoid;
                      }
                      .diff-added {
                       background: rgba(232, 243, 255, 1);
                     
                      }
                      .diff-removed {
                       background: rgba(255, 206, 206, 1);
                    
                      }
                      .diff-modified {
                       background: rgba(250, 223, 170, 1);
                    
                      }
                      /* 词语级修改的高亮样式 */
                      .diff-modified-word {
                       background: rgba(255, 195, 0, 0.4);
                       text-decoration: underline;
                      }
                     
                      table {
                        width: 100%;
                        border-collapse: collapse;
                        margin: 15px 0;
                      }
                      td, th {
                        border: 1px solid #ddd;
                        padding: 12px;
                        text-align: left;
                      }
                      h1, h2, h3, h4 {
                        margin: 20px 0 10px 0;
                        color: #333;
                      }
                    </style>
                  </head>
                  <body>
                    <div class="docx-container" id="doc-container"></div>
                  </body>
                </html>
      `;
    });
    
    // 获取iframe的document对象和容器
    const iframeDoc = iframe.contentDocument || iframe.contentWindow?.document;
    if (!iframeDoc) {
      throw new Error('无法访问iframe的document对象');
    }
    const container = iframeDoc.getElementById('doc-container');
    if (!container) {
      throw new Error('无法找到iframe中的文档容器');
    }
    
    // 创建样式容器
    const styleContainer = iframeDoc.createElement('div');
    iframeDoc.head.appendChild(styleContainer);
    
    // 获取文档数据
    const response = await fetch(resolvedFileUrl);
    const arrayBuffer = await response.arrayBuffer();
    
    // 动态导入docx-preview并渲染文档到iframe中
    const docx = await import('docx-preview');
    await docx.renderAsync(arrayBuffer, container, styleContainer, {
      className: "docx-content",
      breakPages: false,           // 保持分页
      inWrapper: true,            // 使用包装器
      ignoreWidth: false,          // 忽略原始宽度，自适应容器
      ignoreHeight: false,        // 保持高度
      ignoreFonts: false,         // 保持字体
      ignoreLastRenderedPageBreak: true,  // 忽略最后的分页符，减少空白页
      experimental: false,        // 关闭实验性功能，提高稳定性
      trimXmlDeclaration: true,
      useBase64URL: true,         // 使用 base64 处理图片
      renderChanges: false,
      renderHeaders: false,       // 不渲染页眉（避免页码问题）
      renderFooters: false,       // 不渲染页脚（避免页码问题）
      renderFootnotes: true,
      renderEndnotes: true,
      renderComments: false,
      renderAltChunks: true,
      debug: false,
    });
    
    // 确保iframe内容完全加载并调整大小
    setTimeout(() => {
      if (iframeDoc && iframeDoc.body) {
        // 移除容器高度限制，允许文档正常滚动
        container.style.height = 'auto';
        container.style.minHeight = '100%';
        
        // 为页面添加适当的边距
        const pages = iframeDoc.querySelectorAll('.docx-page');
        
        // 隐藏所有页码显示（因为页码不准确）
        const docxWrappers = iframeDoc.querySelectorAll('.docx-wrapper > .docx');
        docxWrappers.forEach((el: Element) => {
          const footer = el.querySelector('footer');
          if (footer) {
            // 隐藏整个 footer 中的页码部分
            const allSpans = footer.querySelectorAll('span');
            allSpans.forEach((span) => {
              const spanText = span.textContent?.trim() || '';
              // 如果是纯数字或包含页码相关文本，隐藏它
              if (/^\d+$/.test(spanText) || /^\d+\s*\/\s*\d+$/.test(spanText) || 
                  spanText.includes('PAGE') || spanText.includes('NUMPAGES')) {
                (span as HTMLElement).style.visibility = 'hidden';
              }
            });
          }
        });
        
        // 处理每个页面的样式，隐藏页码
        pages.forEach((page: any) => {
          page.style.margin = '40px auto';
          page.style.padding = '60px 40px';
          page.style.position = 'relative';
          page.style.boxShadow = '0 1px 3px rgba(0,0,0,0.1)';
          
          // 隐藏页面内 footer 中的页码
          const footer = page.querySelector('footer, .docx-footer');
          if (footer) {
            const pageNumberSpans = footer.querySelectorAll('span');
            pageNumberSpans.forEach((span: HTMLElement) => {
              const text = span.textContent?.trim() || '';
              // 隐藏页码相关的元素
              if (/^\d+$/.test(text) || /^\d+\s*\/\s*\d+$/.test(text) ||
                  text.includes('PAGE') || text.includes('NUMPAGES')) {
                span.style.visibility = 'hidden';
              }
            });
          }
        });
        
        // 动态检测实际页数并更新状态
        const actualPageCount = pages.length || 1;
        console.log(`检测到实际页数: ${actualPageCount}`);
        
        // 更新文档的实际页数到状态中
        if (containerRef === originalDocRef) {
          console.log(`标准文档实际页数: ${actualPageCount}`);
        } else if (containerRef === comparisonDocRef) {
          console.log(`对比文档实际页数: ${actualPageCount}`);
        }
        
        // 文档渲染完成后，应用差异高亮
        applyDiffHighlighting(iframeDoc, containerRef === originalDocRef);
        
        iframe.contentWindow?.scrollTo(0, 0);
        
        // 在这里应用初始缩放
        const zoomLevel = isOriginal ? originalZoomLevel : comparisonZoomLevel;
        applyZoom(iframeRef, zoomLevel);
        
        // 确保文档正确显示并修复滚动条问题
        setTimeout(() => {
          const originalIframe = originalIframeRef.current;
          const comparisonIframe = comparisonIframeRef.current;
          
          if (originalIframe && comparisonIframe) {
            const originalDoc = originalIframe.contentDocument;
            const comparisonDoc = comparisonIframe.contentDocument;
            
            if (originalDoc && comparisonDoc) {
              // 获取iframe容器的高度
              const containerHeight = originalIframe.offsetHeight;
              
              // 确保iframe内部文档高度正确
              originalDoc.documentElement.style.height = 'auto';
              originalDoc.body.style.height = 'auto';
              comparisonDoc.documentElement.style.height = 'auto';
              comparisonDoc.body.style.height = 'auto';
              
              // 隐藏iframe内部的滚动条
              originalDoc.documentElement.style.overflowY = 'hidden';
              comparisonDoc.documentElement.style.overflowY = 'hidden';
              
              // 设置iframe的高度为各自实际内容高度
              const originalContentHeight = Math.max(originalDoc.documentElement.scrollHeight, originalDoc.body.scrollHeight);
              const comparisonContentHeight = Math.max(comparisonDoc.documentElement.scrollHeight, comparisonDoc.body.scrollHeight);
              originalIframe.style.height = `${originalContentHeight}px`;
              comparisonIframe.style.height = `${comparisonContentHeight}px`;
              
              console.log(`标准文档内容高度: ${originalContentHeight}px，对比文档内容高度: ${comparisonContentHeight}px`);
            }
          }
          
          // 文档完全加载后，强制同步一次滚动位置
          if (syncScroll) {
            alignDocumentPositions();
          }
        }, 300);
      }
    }, 500); 
  } catch (error) {
    console.error('渲染文档失败:', error);
    if (containerRef.current) {
      containerRef.current.innerHTML = '<div class="text-red-500 text-center py-10">文档渲染失败</div>';
    }
  }
};
  
  // 当文档URL变化或组件挂载时渲染文档
  useEffect(() => {
    // 使用setTimeout确保DOM已经挂载完成
    const timer = setTimeout(() => {
      console.log('渲染文档 - originalFile:', originalFile);
      console.log('渲染文档 - comparisonFile:', comparisonFile);
      console.log('originalDocRef:', originalDocRef.current);
      console.log('comparisonDocRef:', comparisonDocRef.current);
      
      // 获取标准文档URL
      const standardFileUrl = originalFile?.original_file_url || originalFile?.file_url;
      if (standardFileUrl && originalDocRef.current) {
        console.log('开始渲染标准文档:', standardFileUrl);
        renderDocument(standardFileUrl, originalDocRef as React.RefObject<HTMLDivElement>, originalIframeRef as React.RefObject<HTMLIFrameElement>,true);
      } else {
        console.log('标准文档渲染条件不满足:', !standardFileUrl ? '缺少文件URL' : '缺少DOM引用');
      }
      
      // 获取对比文档URL
      const comparisonFileUrl = comparisonFile?.comparison_file_url || comparisonFile?.file_url;
      if (comparisonFileUrl && comparisonDocRef.current) {
        console.log('开始渲染对比文档:', comparisonFileUrl);
        renderDocument(comparisonFileUrl, comparisonDocRef as React.RefObject<HTMLDivElement>, comparisonIframeRef as React.RefObject<HTMLIFrameElement>,false);
      } else {
        console.log('对比文档渲染条件不满足:', !comparisonFileUrl ? '缺少文件URL' : '缺少DOM引用');
      }
    }, 100);
    
    return () => clearTimeout(timer);
  }, [originalFile, comparisonFile]);

  // 处理结果项点击
  const handleResultClick = (result: ComparisonItem) => {
    setSelectedResult(result);
    
    // 解析位置信息，提取段落索引
    const positionMatch = result.position.match(/第(\d+)段/);
    if (!positionMatch) {
      console.log('无法解析位置信息:', result.position);
      return;
    }
    
    const paragraphIndex = parseInt(positionMatch[1]) - 1;
    
    
    const originalIframe = originalIframeRef.current;
    const comparisonIframe = comparisonIframeRef.current;
    
    if (!originalIframe || !comparisonIframe) {
      console.log('无法获取iframe');
      return;
    }
    
    const originalDoc = originalIframe.contentDocument;
    const comparisonDoc = comparisonIframe.contentDocument;
    
    if (!originalDoc || !comparisonDoc) {
      console.log('无法获取iframe文档');
      return;
    }
    
    // 获取两边文档的所有段落
    const originalParagraphs = originalDoc.querySelectorAll('.docx-paragraph, p');
    const comparisonParagraphs = comparisonDoc.querySelectorAll('.docx-paragraph, p');
    
    // 根据结果类型处理定位和高亮
    if (result.type === 'modified') {
      // 对于修改类型的结果，同时定位到两边文档
      
      // 滚动到原始文档的目标段落
      if (paragraphIndex >= 0 && paragraphIndex < originalParagraphs.length) {
        const originalTarget = originalParagraphs[paragraphIndex] as HTMLElement;
        originalTarget.scrollIntoView({ behavior: 'auto', block: 'center' });
        
        // 高亮原始文档的目标段落
        originalTarget.classList.add('bg-yellow-100');
        setTimeout(() => {
          originalTarget.classList.remove('bg-yellow-100');
        }, 2000);
      }
      
      // 滚动到对比文档的目标段落
      if (paragraphIndex >= 0 && paragraphIndex < comparisonParagraphs.length) {
        const comparisonTarget = comparisonParagraphs[paragraphIndex] as HTMLElement;
        comparisonTarget.scrollIntoView({ behavior: 'auto', block: 'center' });
        
        // 高亮对比文档的目标段落
        comparisonTarget.classList.add('bg-yellow-100');
        setTimeout(() => {
          comparisonTarget.classList.remove('bg-yellow-100');
        }, 2000);
      }
    } else {
      // 对于其他类型结果，只定位到一边文档
      const isOriginalDoc = result.type === 'deleted';
      
      if (isOriginalDoc) {
        // 处理删除类型，定位到原始文档
        if (paragraphIndex >= 0 && paragraphIndex < originalParagraphs.length) {
          const originalTarget = originalParagraphs[paragraphIndex] as HTMLElement;
          originalTarget.scrollIntoView({ behavior: 'auto', block: 'center' });
          
          // 高亮目标段落
          originalTarget.classList.add('bg-yellow-100');
          setTimeout(() => {
            originalTarget.classList.remove('bg-yellow-100');
          }, 2000);
        }
      } else {
        // 处理增加类型，定位到对比文档
        if (paragraphIndex >= 0 && paragraphIndex < comparisonParagraphs.length) {
          const comparisonTarget = comparisonParagraphs[paragraphIndex] as HTMLElement;
          comparisonTarget.scrollIntoView({ behavior: 'auto', block: 'center' });
          
          // 高亮目标段落
          comparisonTarget.classList.add('bg-yellow-100');
          setTimeout(() => {
            comparisonTarget.classList.remove('bg-yellow-100');
          }, 2000);
        }
      }
    }
  };

  // 处理返回按钮点击
  const handleBack = () => {
    router.push('/');
  };

  // 获取结果项的显示样式
  const getResultItemStyle = (type: string) => {
    switch (type) {
      case 'added':
        return 'bg-green-50 border-l-4 border-green-500';
      case 'deleted':
        return 'bg-red-50 border-l-4 border-red-500';
      case 'modified':
        return 'bg-yellow-50 border-l-4 border-yellow-500';
      default:
        return 'border-l-4 border-gray-500';
    }
  };

  // 从ContrastuploadStore获取状态管理方法
  const { setOriginalFile, setComparisonFile } = ContrastuploadStore();

  // Tab 对应的路由路径
  const tabRoutes: Record<TabType, string> = {
    check: '/',
    contrast: '/contrast',
    history: '/history',
    databoard: '/databoard',
  };

  // 处理标签页切换
  const handleTabChange = (tab: TabType) => {
    setActiveTab(tab);
    
  
    if (originalFile?.file_url) {
      setOriginalFile({
        title: originalFile.title,
        file_url: originalFile.file_url,
        file_id: originalFile.file_id
      });
    }
    
    if (comparisonFile?.file_url) {
      setComparisonFile({
        title: comparisonFile.title,
        file_url: comparisonFile.file_url,
        file_id: comparisonFile.file_id
      });
    }
    
    // 跳转到对应的页面
    router.push(tabRoutes[tab]);
  };

  return (
    <div className="flex flex-col h-screen">
      {/* 顶部导航栏 */}
      <Topbar 
        user={user}
        onLoginClick={handleLoginClick}
        onLogoutClick={handleLogout}
        activeTab={activeTab} 
      />
      
      {/* 文档操作工具栏 */}
      <div className="bg-[#f5f5f5] border-b border-gray-200 px-4 py-2 flex items-center gap-2">
        
        <Button type="text" className="text-[#333] text-[14px] flex items-center " onClick={() => {
          // 清除两个文档的URL和其他数据
          ContrastuploadStore.getState().resetAll();
          // 清除localStorage中的相关数据
          localStorage.removeItem('original_file_url');
          localStorage.removeItem('comparison_file_url');
          localStorage.removeItem('original_file_id');
          localStorage.removeItem('comparison_file_id');
          localStorage.removeItem('original_file_title');
          localStorage.removeItem('comparison_file_title');
          // 跳转到对比上传页面
          router.push('/contrast');
        }}>
          <img src="/Retrun.svg" alt="返回" className="w-3 h-3 cursor-pointer" />
          返回
        </Button>
        <div className="flex items-center gap-2">
          <span className="text-[14px] text-[#333]">同步滚动</span>
          <div 
            className={`w-10 h-5 rounded-full transition-colors duration-300 ease-in-out cursor-pointer ${syncScroll ? 'bg-[rgba(39,102,255,1)]' : 'bg-gray-300'}`}
            onClick={() => setSyncScroll(!syncScroll)}
            style={{ position: 'relative' }}
          >
            <div 
              className={`w-4 h-4 bg-white rounded-full transition-transform duration-300 ease-in-out ${syncScroll ? 'transform translate-x-5 -translate-y-1/2' : 'transform translate-x-0.5 -translate-y-1/2'}`}
              style={{ position: 'absolute', top: '50%' }}
            />
          </div>
        </div>
      
      </div>
      
      {/* 主内容区域 */}
      <div className="flex-1 flex overflow-hidden">
        {/* 左侧文档对比区域 */}
        <div className="flex-1 flex overflow-hidden">
          {/* 标准文档 */}
          <div className="w-1/2 h-full bg-white border-r border-gray-200 overflow-hidden">
            {/* 文档控制栏 */}
            <div className="flex items-center justify-between px-4 h-12 border-b border-gray-200 bg-[rgba(255, 255, 255, 1)]">
              <div className="flex items-center gap-2">
                <Button
                  type="text"
                  size="small"
                  className="font-semibold  text-[#333]"
                  style={{
                    borderRadius: 3,
                    display: 'flex',
                    justifyContent: 'center',
                    alignItems: 'center',
                    padding: '4px 12px 4px 12px',
                    backgroundColor: 'rgba(34, 96, 242, 1)',
                    color: '#fff',
                    height: '26px',
                    fontSize: '14px',
                  }}
                >
                  标准文档
                </Button>
                <span className="text-[14px] text-[#666]">{originalFile?.original_file_title || originalFile?.title || '未命名文档'}</span>
              </div>
              <div className="flex items-center gap-2">
                <Button type="text" size="small" className="w-6 h-6 flex items-center justify-center p-0 contract-result-zoom-btn" onClick={handleOriginalZoomOut}>
                  <img src="/4.svg" className="w-full h-full" />
                </Button>
                <span className="text-[14px] text-[#666]">缩放 {originalZoomLevel}%</span>
                <Button type="text" size="small" className="w-6 h-6 flex items-center justify-center p-0 contract-result-zoom-btn" onClick={handleOriginalZoomIn}>
                  <img src="/5.svg" className="w-full h-full" />
                </Button>
                
                
              </div>
            </div>
            {/* 文档内容区域 */}
            <div ref={originalScrollContainerRef} className="h-[calc(100%-45px)] overflow-y-auto" style={{ minHeight: '600px' }}>
              {/* 使用ref渲染文档 */}
              <div ref={originalDocRef} className="docx-render-container w-full bg-[#f0f0f0] text-center">
                <div className="text-center text-gray-400 py-10">
                  正在加载标准文档...
                </div>
              </div>
            </div>
           
          </div>
          
          {/* 对比文档 */}
          <div className="w-1/2 h-full bg-white overflow-hidden">
            {/* 文档控制栏 */}
            <div className="flex items-center justify-between px-4 h-12 border-b border-gray-200 bg-[rgba(255, 255, 255, 1)]">
              <div className="flex items-center gap-2">
                <Button type="text" size="small" className="font-semibold  text-[#333]" style={{
                  borderRadius: 3,
                  display: 'flex',
                  justifyContent: 'center',
                  alignItems: 'center',
                  padding: '4px 12px 4px 12px',
                  backgroundColor: 'rgba(128, 128, 128, 1)',
                  color: '#fff',
                  height: '26px',
                  fontSize: '14px',
                }}>对比文档</Button>
                <span className="text-xs text-[#666]">{comparisonFile?.title || '未命名文档'}</span>
              </div>
              <div className="flex items-center gap-2">
                <Button type="text" size="small" className="w-6 h-6 flex items-center justify-center p-0 contract-result-zoom-btn" onClick={handleComparisonZoomOut}>
                  <img src="/4.svg" className="w-full h-full" />
                </Button>
                <span className="text-[14px] text-[#666]">缩放 {comparisonZoomLevel}%</span>
                <Button type="text" size="small" className="w-6 h-6 flex items-center justify-center p-0 contract-result-zoom-btn" onClick={handleComparisonZoomIn}>
                  <img src="/5.svg" className="w-full h-full" />
                </Button>
                
                
              </div>
            </div>
            
            {/* 文档内容区域 */}
            <div ref={comparisonScrollContainerRef} className="h-[calc(100%-46px)] overflow-y-auto" style={{ direction: 'rtl' }}>
              {/* 使用ref渲染文档 */}
              <div ref={comparisonDocRef} className="docx-render-container w-full bg-[#f0f0f0]" style={{ direction: 'ltr' }}>
                <div className="text-center text-gray-400 py-10">
                  正在加载对比文档...
                </div>
              </div>
            </div>
           
          </div>
        </div>
        
        {/* 右侧对比结果区域 */}
        <div className="w-110 h-full bg-white border-l border-gray-200 flex flex-col">
          {/* 结果统计菜单 */}
          <div className="p-2 border-b border-gray-200  relative  h-12 bg-[rgba(255, 255, 255, 1)]">
            <div ref={menuContainerRef} className="flex items-center overflow-x-auto scrollbar-hide">
              <button 
                ref={allButtonRef}
                className={`px-2 py-1 rounded-full text-[14px] font-medium transition-colors whitespace-nowrap ${
                  activeMenu === '全部' ? 'text-[rgba(39,102,255,1)]' : 'text-gray-500 hover:text-[rgba(39,102,255,1)]'
                }`}
                onClick={() => setActiveMenu('全部')}
              >
                全部 ({resultStats.total})
              </button>
              <button 
                ref={addedButtonRef}
                className={`px-2 py-1 rounded-full text-[14px] font-medium transition-colors whitespace-nowrap ${ 
                  activeMenu === '增加' ? 'text-[rgba(39,102,255,1)]' : 'text-gray-500 hover:text-[rgba(39,102,255,1)]'
                }`}
                onClick={() => setActiveMenu('增加')}
              >
                增加 ({resultStats.added})
              </button>
              <button 
                ref={deletedButtonRef}
                className={`px-2 py-1 rounded-full text-[14px] font-medium transition-colors whitespace-nowrap ${   
                  activeMenu === '删除' ? 'text-[rgba(39,102,255,1)]' : 'text-gray-500 hover:text-[rgba(39,102,255,1)]'
                }`}
                onClick={() => setActiveMenu('删除')}
              >
                删除 ({resultStats.deleted})
              </button>
              <button 
                ref={modifiedButtonRef}
                className={`px-3 py-1 rounded-full text-[14px] font-medium transition-colors whitespace-nowrap ${activeMenu === '修改' ? 'text-[rgba(39,102,255,1)]' : 'text-gray-500 hover:text-[rgba(39,102,255,1)]'}`}
                onClick={() => setActiveMenu('修改')}
              >
                修改 ({resultStats.modified})
              </button>
              <div style={{
                width: '1px',
                height: '25px',
                backgroundColor:'rgba(209, 209, 209, 1)',
                marginLeft: '4px',
                marginRight: '4px',
              }}></div>
              <button 
                ref={ignoredButtonRef}
                className={`px-3 py-1 rounded-full text-[14px] font-medium transition-colors whitespace-nowrap ${activeMenu === '忽略' ? 'text-[rgba(39,102,255,1)]' : 'text-gray-500 hover:text-[rgba(39,102,255,1)]'}`}
                onClick={() => setActiveMenu('忽略')}
              >
                忽略 ({resultStats.ignored})
              </button>
            </div>
            {/* 蓝色滑动段线 */}
            <div 
              className="absolute bottom-0 left-0 h-0.5 bg-[rgba(39,102,255,1)] transition-all duration-300 ease-in-out"
              style={{
                width: 20, // 固定宽度为20px
                transform: `translateX(${activeMenuPosition}px)`
              }}
            ></div>
          </div>
          
          {/* 结果列表 */}
          <div className="flex-1 overflow-y-auto p-2" style={{backgroundColor: 'rgba(243, 244, 246, 1)'}}>
            {/* 加载中状态 */}
            {loading && (
              <div className="flex justify-center items-center h-full">
                <div className="text-center">
                  <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-2"></div>
                  <p className="text-gray-500">正在分析对比结果...</p>
                </div>
              </div>
            )}
            
            {/* 错误状态 */}
            {!loading && error && (
              <div className="flex justify-center items-center h-full">
                <div className="text-center">
                  <div className="text-red-500 text-4xl mb-2">⚠️</div>
                  <p className="text-red-500 mb-2">{error}</p>
                  <Button 
                    type="primary" 
                    size="small" 
                    onClick={() => window.location.reload()}
                  >
                    重试
                  </Button>
                </div>
              </div>
            )}
            
            {/* 空数据状态 */}
            {!loading && !error && comparisonResults.length === 0 && (
              <div className="flex justify-center items-center h-full">
                <p className="text-gray-400">未发现差异</p>
              </div>
            )}
            
            {/* 结果列表 */}
            {!loading && !error && comparisonResults.length > 0 && (
              comparisonResults
                .filter(result => {
                  // 如果是"忽略"标签页，只显示被忽略的结果
                  if (activeMenu === '忽略') return result.isIgnored;
              
                  if ((result as { isIgnored?: boolean }).isIgnored) return false;
          
                  if (activeMenu === '全部') return true;
                  if (activeMenu === '增加') return result.type === 'added';
                  if (activeMenu === '删除') return result.type === 'deleted';
                  if (activeMenu === '修改') return result.type === 'modified';
                  return true;
                })
                .map((result) => (
                  <div
                    key={result.id}
                    className={`p-3 mb-2 rounded-md duration-300 bg-white border relative ${selectedResult?.id === result.id ? 'border-blue-500' : 'border-gray-100'}`}
                    onClick={() => handleResultClick(result)}
                  >
                    {/* 左侧黑色竖线标识 */}
                    <div className="absolute left-[12px] top-[12px] h-[25px] transition-all duration-300" style={{
                      backgroundColor: TypeColorMap[result.type],
                      width: 5
                    }}></div>
                    
                  {/* 右上角忽略/取消忽略按钮 */}
                  <button style={{fontSize: '14px,font-weight: 400',letterSpacing: '0px',lineHeight: '20.27px'}}
                    className="absolute right-[24px] top-[16px]  text-blue-500 hover:text-blue-700 transition-all duration-300" 
                    onClick={(e) => {
                      e.stopPropagation();
                      if (result.isIgnored) {
                   
                        setComparisonResults(prevResults => 
                          prevResults.map(item => 
                            item.id === result.id ? { ...item, isIgnored: false } : item
                          )
                        );
                      } else {
                        setComparisonResults(prevResults => 
                          prevResults.map(item => 
                            item.id === result.id ? { ...item, isIgnored: true } : item
                          )
                        );
                      }
                    }}
                  >
                    {result.isIgnored ? '取消忽略' : '忽略'}
                  </button>
                  
                  <div className="pl-4">
                    <div className="  mb-2" style={{fontSize: '18px', fontWeight: 700, color: 'rgba(56, 56, 56, 1) top-[12px]'}}>
                      {result.type === 'added' ? '新增' : result.type === 'deleted' ? '删除' : '修改'}
                    </div>
                    <div className=" text-gray-600 leading-relaxed" style={{fontSize: '16px,font-weight: 400',lineHeight: '24px'}}>
                      {result.type === 'added' && (
                        <>
                          标准合同：
                          <br />
                          <span >
                            {result.content.replace('新增条款：', '')}
                          </span>
                        </>
                      )}
                      {result.type === 'deleted' && result.content}
                      {result.type === 'modified' && (
                        <>
                          标准合同：
                          <br />
                          <span style={{fontSize: '16px,font-weight: 400',lineHeight: '24px'}}>
                            {result.original.replace(/标准合同：\s*/, '')}
                          </span>
                          <br />
                          <div style={{ 
                            width: 'calc(98%)', 
                            height: '0px', 
                            opacity: 1, 
                            border: '1px solid rgba(229, 229, 229, 1)', 
                            
                            marginBottom: '10px',
                            marginTop: '10px',
                          }}></div>
                           对比合同：
                           <br/>
                          <span style={{ color: 'rgba(34, 96, 242, 1)',fontSize: '16px,font-weight: 400 ',lineHeight: '24px' }}>
                            {result.modified.replace('修改为：', '')}
                          </span>
                        </>
                      )}
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
          
          {/* 底部操作按钮 */}
        
        </div>
      </div>

      {/* 登录模态框 */}
      <LoginModal
        visible={loginVisible}
        onCancel={() => setLoginVisible(false)}
        onSuccess={handleLoginSuccess}
        onSwitchToRegister={handleSwitchToRegister}
      />

      {/* 注册模态框 */}
      <RegisterModal
        visible={registerVisible}
        onCancel={() => setRegisterVisible(false)}
        onSuccess={handleRegisterSuccess}
        onSwitchToLogin={handleSwitchToLogin}
      />
    </div>
  );
}
