'use client';
import React, { useEffect, useRef } from 'react';
import * as echarts from 'echarts';
import { signboardMock } from '@/mock/mock';

const TrendChart: React.FC = () => {
  const chartRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!chartRef.current) return;
    const chart = echarts.init(chartRef.current);
    const option = {
      title: { text: '合同审阅趋势', left: '0rem', top: '2rem',bottom:'0px', textStyle: { color: 'rgba(0, 0, 0, 1)', fontSize: '1.13rem' } },
      legend: {
        data: ['合同总数', '服务类合同', '货物类合同', '基建类合同'],
        top: '4rem', left: '125rem', textStyle: { fontSize: '0.88rem' }
      },
      tooltip: {
        trigger: 'axis',
        axisPointer: { type: 'shadow' }
      },
     grid: { left: '50rem', bottom: '40rem', right: '80px' ,top:'70rem'},
      // X轴（月份）
      xAxis: {
        type: 'category',
        data: signboardMock.trendChart.xAxis,
        axisLine: { show: true ,style:'1px solid rgba(227, 227, 227, 1)'},
        axisTick: { show: false  },
        axisLabel: {
              margin: 20,
    color: 'rgba(77, 77, 77, 1)',
    fontSize: 14
  }
      },
      yAxis: [
        // 左侧Y轴（合同数量）
        {
          type: 'value',
          min: 0,
          max: 3500,
          interval:500,
          
          splitLine: {
            lineStyle: { type: 'dashed' }
          },
    
           axisLabel: {
            formatter: '{value}',
    align:'center',  
    margin:25,      
    color: 'rgba(77, 77, 77, 1)',
    fontSize: 14
  }
        },
        // 右侧Y轴（折线对应数值）
        {
          type: 'value',
          min: 0,
          max: 1400,
          interval:200,
          position: 'right',
          axisLine: { show: false },
          axisTick: { show: false },
          splitLine: { show: false },
          axisLabel: {
            formatter: '{value}',
    align:'center',  
    margin:28,      
    color: 'rgba(77, 77, 77, 1)',
    fontSize: 14
  }
        }
      ],
      // 系列（柱状图+折线图）
      series: [
        // 柱状图（合同总数）
        {
          name: '合同总数',
          type: 'bar',
          data: signboardMock.trendChart.yAxisData1,
          barWidth: 18,
          itemStyle: { color: 'rgba(34, 96, 242, 1)' }
        },
        // 折线图（对应右侧Y轴）
        {
          name: '服务类合同',
          type: 'line',
          yAxisIndex: 1, 
          data: signboardMock.trendChart.yAxisData2,
          symbol: 'circle',
          symbolSize: 6,
          lineStyle: { color: 'rgba(0, 186, 173, 1)' },
          itemStyle: { color: 'rgba(242, 242, 242, 1)' }
        }
      ]
    };

    chart.setOption(option);
    const handleResize = () => chart.resize();
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      chart.dispose();
    };
  }, []);

  return (

    <div
      ref={chartRef}
      style={{ width: '100%', height: '100%' }}
    />
  );
};

export default TrendChart;