'use client';
import React, { useEffect, useRef } from 'react';
import type { ECharts } from 'echarts';
import { WordCloudItem } from '@/lib/Interface';
import * as echarts from 'echarts';
import { signboardMock } from '@/mock/mock';

const WordCloudChart = ({ data }: { data: WordCloudItem[] }) => {
  const wordChartRef = useRef<HTMLDivElement>(null);
  let myChart: ECharts | null = null; 

  useEffect(() => {
    if (data.length === 0) return;

    const initChart = () => {
      if (typeof window === 'undefined' || !wordChartRef.current) return;
      const existingChart = echarts.getInstanceByDom(wordChartRef.current);
      if (existingChart) {
        existingChart.dispose();
      }

      myChart = echarts.init(wordChartRef.current);
 
      const option = {
        tooltip: {
          trigger: 'item',
        },
        series: [
          {
            type: 'wordCloud',
            shape: 'rect', // 使用默认形状：circle（圆形）、cardioid（心形）、diamond（菱形）、triangle（三角形）、pentagon（五边形）、star（星形）
            left: 'center',
            top: '40px',
            width: '80%',
            height: '80%',
            sizeRange: [12, 60],
            rotationRange: [0, 0],
            gridSize: 10,
            drawOutOfBound: false,
            data: signboardMock.wordCloudData,
            textStyle: {
              color: function () {
                const colors = ['#2260F2', '#ED8C2B', '#FF5733', '#ba9292ff'];
                return colors[Math.floor(Math.random() * colors.length)];
              },
              emphasis: {
                shadowBlur: 10,
                shadowColor: '#333',
              },
            },
          },
        ],
      };
      if (myChart) {
        myChart.setOption(option);
      }
    };

    import('echarts-wordcloud').then(() => {
      initChart();
    }).catch(err => {
      console.error('echarts-wordcloud 加载失败:', err);
    });

    const handleResize = () => {
      myChart?.resize();
    };

    window.addEventListener('resize', handleResize);

    return () => {
      myChart?.dispose();
      window.removeEventListener('resize', handleResize);
    };
  }, [data]); 
  
  return (
    <div style={{ width: '100%', height: '100%', position: 'relative' }}>
      <div
        ref={wordChartRef}
        style={{
          width: '100%',
          height: '100%',
          borderRadius: '4px',
        }}
      />
    </div>
  );
};

export default WordCloudChart;