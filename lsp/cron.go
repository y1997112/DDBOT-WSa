package lsp

import (
	"fmt"
	"sync"

	"github.com/Sora233/DDBOT/lsp/cfg"
	"github.com/Sora233/DDBOT/lsp/mmsg"
	"github.com/Sora233/DDBOT/lsp/template"
	"github.com/sirupsen/logrus"
)

var cronLog = logrus.WithField("module", "cronjob")

type cronjobRun struct {
	*cfg.CronJob
	l *Lsp
}

func (c *cronjobRun) Run() {
	templateName := fmt.Sprintf("custom.cronjob.%s.tmpl", c.TemplateName)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for _, groupCode := range c.Target.Group {
			m, _ := template.LoadAndExec(templateName, map[string]interface{}{
				"target": groupCode,
			})
			if m != nil {
				// fmt.Printf("插件的发送分发区:")
				// elements := m.Elements() // 调用函数来获取Elements切片
				// // 打印elements的数量
				// fmt.Printf("Number of Elements in m: %d\n", len(elements))
				// // 打印每个Element的类型
				// for i, elem := range elements {
				// 	fmt.Printf("Element %d is of type %T\n", i, elem)
				// }
				c.l.SendMsg(m, mmsg.NewGroupTarget(groupCode))
			}
		}
	}()
	go func() {
		defer wg.Done()
		for _, uin := range c.Target.Private {
			m, _ := template.LoadAndExec(templateName, map[string]interface{}{
				"target": uin,
			})
			if m != nil {
				c.l.SendMsg(m, mmsg.NewPrivateTarget(uin))
			}
		}
	}()
	wg.Wait()
}

func (l *Lsp) CronjobReload() {
	for _, entry := range l.cron.Entries() {
		l.cron.Remove(entry.ID)
	}
	cronjobs := cfg.GetCronJob()
	for _, entry := range cronjobs {
		if _, err := l.cron.AddJob(entry.Cron, &cronjobRun{entry, l}); err != nil {
			cronLog.WithField("cron_exp", entry.Cron).
				WithField("template_name", entry.TemplateName).
				WithField("target_group", entry.Target.Group).
				WithField("target_private", entry.Target.Private).
				Errorf("添加定时任务失败：%v", err)
		}
	}
}

func (l *Lsp) CronStart() {
	l.cron.Start()
}

func (l *Lsp) CronStop() {
	<-l.cron.Stop().Done()
}
