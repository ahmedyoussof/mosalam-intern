# التاسك الاولانية يا شباب:

اقرأ التعليمات كويس وحاول تفهمها وتدور عليها باكبر عمق ممكن وخصوصا اخر 5 طلبات تحديدا،
 واحنا هنعمل سيشن بعدها نستضيف 5 منكم هيعمل شار سكرين و نستعرض شغلهم ونصحح الاخطاء والمفاهيم لو فيه حاجه مش مفهومة ، 
ونجاوب علي الاسئلة ، والسيشن هيبقي مسجل واللي هيحضر اونلين هيبقي ليه الاولوية في عرض شغله.
 
---
نزل دوكر انجين
إضافة المستخدم لمجموعة docker — الشرط: تقدر تشغّل docker ps من غير sudo.


- قمت بـ install لدوكر انجين:  رجوعا الي ال documentation الخاصه بـ installation فوجدت انه لابد من حذف كل ما هو متعلق بدوكر لتجنب المشاكل 

```
sudo apt remove $(dpkg --get-selections docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc | cut -f1)



# Add Docker's official GPG key:
sudo apt update
sudo apt install ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

# Add the repository to Apt sources:
sudo tee /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: $(. /etc/os-release && echo "${UBUNTU_CODENAME:-$VERSION_CODENAME}")
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF


sudo apt update

```

- بعد كدا بدأنا نثبت الدوكر ونتأكد انها هيبقا enabled  + active عشان يكون شغال حتي بعد restart

```
sudo apt install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

sudo systemctl status docker

sudo systemctl start docker

```

---
تفعيل وتشغيل Docker عبر systemd — الشرط: أمر systemctl status docker يظهر Active (running).

إنشاء ملف daemon.json — الشرط: Docker يعمل Restart بدون Errors في journalctl -u docker.

تشغيل أول Container — الشرط: رسالة “Hello from Docker!” تظهر كاملة.

تشغيل 3 Containers مختلفة — الشرط: docker ps يعرض 3 Containers شغّالين بدون Exit.

الملحق: 

فحص Docker Info — الشرط: تعرف تحدد Storage Driver + Cgroup Driver وتكتبهم.

إدارة Images — الشرط: تعمل Pull + Tag + Remove + Inspect لثلاث Images مختلفة.

إدارة Containers — الشرط: تعمل Start/Stop/Restart + Logs + Exec على Container واحد على الأقل.

تغيير Default Log Driver — الشرط: docker info يعرض الـ Log Driver الجديد.

تغيير Network Mode وتشغيل Container عليه — الشرط: Container يشتغل على Network جديدة وتظهر في docker network ls.

وطبعا اللي هيعرف يزود علي الكلام دا او يطلع حاجة ابداعية هينضم لادمن الجروب.
الميتنج هيبقي بكرة الظهر الساعة 12:30 علشان الناس اللي هتنفذ التاسك وتشرحه لينا اونلين .