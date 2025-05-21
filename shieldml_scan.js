const uploadArea = document.getElementById('uploadArea');
const fileInput = document.getElementById('fileInput');
const fileList = document.getElementById('fileList');
const scanBtn = document.getElementById('scanBtn');
const resultArea = document.getElementById('resultArea');
const loading = document.getElementById('loading');
const processedCount = document.getElementById('processedCount');
const totalCount = document.getElementById('totalCount');
let files = [];

// 拖拽上传
uploadArea.addEventListener('click', () => fileInput.click());
uploadArea.addEventListener('dragover', e => {
    e.preventDefault();
    uploadArea.classList.add('dragover');
});
uploadArea.addEventListener('dragleave', e => {
    e.preventDefault();
    uploadArea.classList.remove('dragover');
});
uploadArea.addEventListener('drop', e => {
    e.preventDefault();
    uploadArea.classList.remove('dragover');
        
    // 添加落下动效
    uploadArea.classList.add('file-dropped');
    setTimeout(() => uploadArea.classList.remove('file-dropped'), 600);
        
    handleFiles(e.dataTransfer.files);
});
fileInput.addEventListener('change', e => handleFiles(e.target.files));

function handleFiles(selectedFiles) {
    // 清空原有文件列表，避免累加
    files = [];
        
    // 过滤大小超过10MB的文件
    const invalidFiles = Array.from(selectedFiles).filter(f => f.size > 10 * 1024 * 1024);
    if (invalidFiles.length > 0) {
        alert(`以下文件超过10MB限制，无法上传:\n${invalidFiles.map(f => f.name).join('\n')}`);
    }
        
    // 添加有效文件
    const validFiles = Array.from(selectedFiles).filter(f => f.size <= 10 * 1024 * 1024);
    files = [...validFiles]; // 直接替换，不是追加
        
    // 限制文件数量为20个
    if (files.length > 20) {
        files = files.slice(0, 20);
        alert('最多只能上传20个文件，超出部分已被忽略');
    }
        
    renderFileList();
    scanBtn.disabled = files.length === 0;
    resultArea.innerHTML = '';
}

function renderFileList() {
    if (files.length === 0) {
        fileList.style.display = 'none';
        return;
    }
        
    fileList.style.display = 'block';
    fileList.innerHTML = files.map((f, index) => {
        const fileExt = f.name.split('.').pop().toLowerCase();
        let fileIconClass = 'bi-file-earmark-text';
            
        // 根据文件类型设置不同图标
        if (fileExt === 'php') fileIconClass = 'bi-filetype-php';
        else if (fileExt === 'jsp' || fileExt === 'jspx') fileIconClass = 'bi-filetype-java';
        else if (fileExt === 'asp') fileIconClass = 'bi-filetype-html';
            
        return `
        <div class="file-item">
            <div class="file-info">
                <span class="file-icon"><i class="bi ${fileIconClass}"></i></span>
                <span class="file-name">${f.name}</span>
                <span class="file-size">(${formatFileSize(f.size)})</span>
            </div>
            <button class="file-remove" data-index="${index}" title="移除文件">
                <i class="bi bi-x"></i>
            </button>
        </div>`;
    }).join('');
        
    // 为删除按钮添加事件
    document.querySelectorAll('.file-remove').forEach(btn => {
        btn.addEventListener('click', e => {
            const index = parseInt(e.target.closest('.file-remove').dataset.index);
            files.splice(index, 1);
            renderFileList();
            scanBtn.disabled = files.length === 0;
        });
    });
}
    
function formatFileSize(bytes) {
    if (bytes < 1024) return bytes + " B";
    else if (bytes < 1048576) return (bytes / 1024).toFixed(1) + " KB";
    else return (bytes / 1048576).toFixed(1) + " MB";
}

scanBtn.addEventListener('click', async () => {
    if (files.length === 0) return;
        
    scanBtn.disabled = true;
    scanBtn.classList.add('scanning');
    scanBtn.innerHTML = '<span>正在检测中...</span>';
    loading.style.display = 'flex';
    resultArea.innerHTML = '';
        
    // 更新进度信息
    totalCount.textContent = files.length;
    processedCount.textContent = '0';
        
    const formData = new FormData();
    files.forEach(f => formData.append('file', f));
        
    try {
        // 使用真实API
        const resp = await fetch('/api/scan', { 
            method: 'POST', 
            body: formData 
        });
        if (!resp.ok) {
            throw new Error(`服务器返回错误: ${resp.status}`);
        }
        const data = await resp.json();
            
        // 等待进度超过85%
        let progress = 0;
        const progressInterval = setInterval(() => {
            if (progress >= 100) {
                clearInterval(progressInterval);
                return;
            }
                
            progress += Math.floor(Math.random() * 10) + 1;
            if (progress > 100) progress = 100;
                
            document.querySelector('.loading-percentage').textContent = progress + '%';
                
            // 随机更新处理文件数
            const processed = Math.min(Math.floor((progress / 100) * files.length), files.length);
            processedCount.textContent = processed;
                
            // 更新检测阶段文本
            if (progress < 30) {
                document.querySelector('.loading-text').textContent = '正在分析文件特征...';
            } else if (progress < 60) {
                document.querySelector('.loading-text').textContent = '应用机器学习模型检测...';
            } else if (progress < 90) {
                document.querySelector('.loading-text').textContent = '执行启发式规则检测...';
            } else {
                document.querySelector('.loading-text').textContent = '生成检测报告...';
            }
                
        }, 200);
            
        // 等待进度超过85%
        while (progress < 85) {
            await new Promise(resolve => setTimeout(resolve, 100));
        }
            
        // 清除进度条更新
        clearInterval(progressInterval);
            
        // 显示100%完成
        document.querySelector('.loading-percentage').textContent = '100%';
        processedCount.textContent = files.length;
        document.querySelector('.loading-text').textContent = '检测完成！';
            
        // 短暂延迟后显示结果
        setTimeout(() => {
            showResults(data.results);
            scanBtn.classList.remove('scanning');
            scanBtn.innerHTML = '<span class="scan-btn-icon"><i class="bi bi-shield-check"></i></span><span>点击检测</span>';
            scanBtn.disabled = false;
            loading.style.display = 'none';
        }, 500);
            
    } catch (e) {
        resultArea.innerHTML = `
        <div class="card">
            <div class="result-header">
                <div class="result-title">检测失败</div>
            </div>
            <div class="risk-danger" style="padding: 15px 0;">
                ${e.message}，请重试。检查网络连接或联系管理员。
            </div>
        </div>`;
        console.error(e);
            
        scanBtn.classList.remove('scanning');
        scanBtn.innerHTML = '<span class="scan-btn-icon"><i class="bi bi-shield-check"></i></span><span>点击检测</span>';
        scanBtn.disabled = false;
        loading.style.display = 'none';
    }
});

function showResults(results) {
    if (results.length === 0) {
        resultArea.innerHTML = `
        <div class="card">
            <div class="risk-unknown" style="text-align:center; padding: 15px 0;">
                未检测到任何结果，请重新上传文件。
            </div>
        </div>`;
        return;
    }
    results.sort((a, b) => {
        const riskOrder = {
            '木马文件': 1,
            '疑似木马': 2,
            '无风险': 3,
            '未知': 4
        };
            
        return riskOrder[a.risk] - riskOrder[b.risk];
    }); 
    // 计算检测统计信息
    const total = results.length;
    const safeCount = results.filter(r => r.risk === '无风险').length;
    const warningCount = results.filter(r => r.risk === '疑似木马').length;
    const dangerCount = results.filter(r => r.risk === '木马文件').length;
        
    let html = `
    <div class="card">
        <div class="result-header">
            <div class="result-title">检测结果</div>
            <div class="result-actions">
                <button class="action-btn" id="downloadBtn">
                    <i class="bi bi-download"></i>
                    <span>下载报告</span>
                </button>
                <button class="action-btn" id="clearBtn">
                    <i class="bi bi-arrow-repeat"></i>
                    <span>重新检测</span>
                </button>
            </div>
        </div>
            
        <div class="result-summary">
            <div class="summary-item summary-safe">
                <div class="summary-icon">
                    <i class="bi bi-check-circle"></i>
                </div>
                <div class="summary-content">
                    <div class="summary-count">${safeCount}</div>
                    <div class="summary-label">安全文件</div>
                </div>
            </div>
            <div class="summary-item summary-warning">
                <div class="summary-icon">
                    <i class="bi bi-exclamation-triangle"></i>
                </div>
                <div class="summary-content">
                    <div class="summary-count">${warningCount}</div>
                    <div class="summary-label">疑似木马</div>
                </div>
            </div>
            <div class="summary-item summary-danger">
                <div class="summary-icon">
                    <i class="bi bi-x-circle"></i>
                </div>
                <div class="summary-content">
                    <div class="summary-count">${dangerCount}</div>
                    <div class="summary-label">木马文件</div>
                </div>
            </div>
        </div>
            
        <div class="result-table-wrapper">
            <table class="result-table">
                <thead>
                    <tr>
                        <th width="30%">文件名</th>
                        <th width="10%">类型</th>
                        <th width="15%">风险等级</th>
                        <th width="25%">说明</th>
                        <th width="10%">MD5</th>
                        <th width="10%">大小</th>
                    </tr>
                </thead>
                <tbody>`;
        
    for (const r of results) {
        // 根据风险类型选择风险类名和图标
        let riskClass = 'risk-unknown';
        let icon = '<i class="bi bi-question-circle"></i>';
        let tagClass = '';
            
        if (r.risk === '无风险') {
            riskClass = 'risk-safe';
            icon = '<i class="bi bi-check-circle"></i>';
            tagClass = 'tag-safe';
        } else if (r.risk === '木马文件') {
            riskClass = 'risk-danger';
            icon = '<i class="bi bi-x-circle"></i>';
            tagClass = 'tag-danger';
        } else if (r.risk === '疑似木马') {
            riskClass = 'risk-warning';
            icon = '<i class="bi bi-exclamation-triangle"></i>';
            tagClass = 'tag-warning';
        }
            
        html += `
        <tr>
            <td>
                <div class="file-name">
                    <span class="file-status-icon ${riskClass}">${icon}</span>
                    <span>${r.filename}</span>
                </div>
            </td>
            <td>${r.type || '-'}</td>
            <td><span class="tag ${tagClass}">${r.risk}</span></td>
            <td class="desc-cell" title="${r.description || ''}">${r.description || '-'}</td>
            <td style="font-size:0.85rem; font-family:monospace;">${r.md5 ? r.md5.substring(0, 8) + '...' : '-'}</td>
            <td>${formatFileSize(r.size || 0)}</td>
        </tr>`;
    }
        
    html += `
                </tbody>
            </table>
        </div>
    </div>`;
        
    resultArea.innerHTML = html;
        
    // 添加下载报告按钮事件
    document.getElementById('downloadBtn').addEventListener('click', () => {
        const reportData = {
            timestamp: new Date().toISOString(),
            summary: {
                total: total,
                safe: safeCount,
                warning: warningCount,
                danger: dangerCount
            },
            results: results
        };
            
        const blob = new Blob([JSON.stringify(reportData, null, 2)], {type: 'application/json'});
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `webshell_scan_report_${formatDate(new Date())}.json`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    });
        
    // 添加重新检测按钮事件
    document.getElementById('clearBtn').addEventListener('click', () => {
        files = [];
        renderFileList();
        scanBtn.disabled = true;
        resultArea.innerHTML = '';
    });
}
    
// 格式化日期为 YYYYMMDD_HHMMSS 格式
function formatDate(date) {
    return date.getFullYear() + 
           padZero(date.getMonth() + 1) + 
           padZero(date.getDate()) + '_' + 
           padZero(date.getHours()) + 
           padZero(date.getMinutes()) + 
           padZero(date.getSeconds());
}
    
function padZero(num) {
    return num.toString().padStart(2, '0');
}